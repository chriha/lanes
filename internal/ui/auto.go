package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/history"
	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/session"
)

// autoEvent is the history event an on_enter start writes before it starts (or queues) the action; it keeps the
// action from firing on the item again, also across restarts, and dates the on_enter_repeat cooldown.
const autoEvent = "auto"

// autoState is the in-memory state of the views' on_enter actions. Only successful full fetches change seen and
// left; a config change resets both, so the next full fetch of each view seeds seen again.
type autoState struct {
	off   bool                       // :auto off: nothing fires, on_enter entries in the queue do not drain
	seen  map[string]map[string]bool // by view name: the item IDs the view listed since the config was applied
	left  map[string]map[string]bool // by view name: seen items a later fetch found gone (on_enter_repeat views)
	queue []autoEntry                // starts held back by max_claude_sessions, oldest first
}

// autoEntry is a start waiting in the queue for a free session: an on_enter start of view, or, with manual set, a
// start the user asked for (an action key, :run, or with resume set gr) over max_claude_sessions.
type autoEntry struct {
	view, action, id string
	manual, resume   bool
	act              config.Action // manual: the action as started, for actions the config does not name (shell)
	it               item.Item     // manual: the item as started, used while its provider no longer lists it
	prov             string        // manual: the provider of the item
}

// autoFetched runs after a successful full fetch of provider prov: it drops queued on_enter items that left their
// view, drains the queue, then fires the on_enter action of each view of prov on its new arrivals.
func (m *Model) autoFetched(prov string) tea.Cmd {
	defer m.refreshQueue()
	keep := m.auto.queue[:0]
	for _, e := range m.auto.queue {
		if _, ok := m.autoLookup(e); ok || e.manual {
			keep = append(keep, e)
		}
	}
	m.auto.queue = keep
	cmds := []tea.Cmd{m.drainAuto()}
	for _, v := range m.views {
		if v.cfg.OnEnter != "" && v.cfg.Provider == prov && v.err == nil {
			cmds = append(cmds, m.autoView(v))
		}
	}
	return tea.Batch(cmds...)
}

// autoView diffs the view's items against the ones seen before and fires its on_enter action on the arrivals
// that are due (see autoDue). The view's first full fetch only seeds what it has seen.
func (m *Model) autoView(v *view) tea.Cmd {
	name := v.cfg.Name
	in := map[string]bool{}
	for _, it := range v.members {
		in[it.ID] = true
	}
	if m.auto.seen == nil {
		m.auto.seen, m.auto.left = map[string]map[string]bool{}, map[string]map[string]bool{}
	}
	seen := m.auto.seen[name]
	if seen == nil {
		m.auto.seen[name] = in
		return nil
	}
	cooldown := v.cfg.OnEnterCooldown()
	left := m.auto.left[name]
	if left == nil {
		left = map[string]bool{}
		m.auto.left[name] = left
	}
	if cooldown > 0 {
		for id := range seen {
			if !in[id] {
				left[id] = true
			}
		}
	}
	var fire []item.Item
	for _, it := range v.members {
		if _, snoozed := m.snoozes[it.ID]; snoozed {
			continue // set aside: not an arrival until its snooze ends, when it counts as one
		}
		switch {
		case !seen[it.ID]:
			seen[it.ID] = true
		case left[it.ID]:
			delete(left, it.ID)
		default:
			continue
		}
		if !m.auto.off && autoDue(session.Key(it.ID), v.cfg.OnEnter, cooldown) {
			fire = append(fire, it)
		}
	}
	return m.autoStart(name, v.cfg.OnEnter, fire)
}

// autoDue says whether the item with history key fires action: never auto-started it, or, with a cooldown, its
// last auto start is older than that.
func autoDue(key, action string, cooldown time.Duration) bool {
	var last time.Time
	for _, e := range history.For(key) {
		if e.Event == autoEvent && e.Action == action {
			last = e.Time
		}
	}
	if last.IsZero() {
		return true
	}
	return cooldown > 0 && clock().Sub(last) > cooldown
}

// autoStart starts action on its, the arrivals of view: items its when skips, busy items and items already running
// or queued for it are left out; the rest are recorded in the history, then started while max_claude_sessions has
// room and queued after that.
func (m *Model) autoStart(view, action string, its []item.Item) tea.Cmd {
	act, ok := m.cfg.Action[action]
	if !ok || len(its) == 0 {
		return nil
	}
	match, _, _ := matchWhen(act, its) // the label gate: a skipped item stays seen and does not fire
	var ready []item.Item
	for _, it := range match {
		if m.autoBlocked(action, act, it) || m.queued(it.ID) != "" {
			continue
		}
		history.Append(history.Event{Time: clock(), Item: session.Key(it.ID), Action: action, Event: autoEvent})
		ready = append(ready, it)
	}
	n := m.autoRoom(act, len(ready))
	for _, it := range ready[n:] {
		m.auto.queue = append(m.auto.queue, autoEntry{view: view, action: action, id: it.ID})
	}
	return m.autoRun(action, act, ready[:n])
}

// dropReason says why a queued start of action on it can no longer run, "" when it can.
func (m *Model) dropReason(action string, act config.Action, it item.Item) string {
	if match, _, _ := matchWhen(act, []item.Item{it}); len(match) != 1 {
		return "no longer matches when"
	}
	if b, busy := m.busy[session.Key(it.ID)]; busy {
		return fmt.Sprintf("busy (%s)", b.action)
	}
	if m.autoBlocked(action, act, it) {
		return "already runs " + action
	}
	return ""
}

// autoBlocked says whether it is busy or runs a live session of action already.
func (m *Model) autoBlocked(action string, act config.Action, it item.Item) bool {
	if _, busy := m.busy[session.Key(it.ID)]; busy {
		return true
	}
	if act.Type == "run" {
		return false // a running headless action holds its item busy
	}
	for _, s := range m.sessionsOf(it) {
		if s.Name == session.Name(action, it.ID) && !s.Dead {
			return true
		}
	}
	return false
}

// autoRoom is how many of n starts of act fit under max_claude_sessions; headless runs do not count.
func (m *Model) autoRoom(act config.Action, n int) int {
	limit := m.cfg.Settings.SessionLimit()
	if act.Type == "run" || limit == 0 {
		return n
	}
	return max(0, min(n, limit-m.liveSessions()))
}

// autoRun starts action on its unattended: no prompt, no jump, the user's marks kept. Its footer notice says the
// start was automatic.
func (m *Model) autoRun(action string, act config.Action, its []item.Item) tea.Cmd {
	if len(its) == 0 {
		return nil
	}
	marks := m.marks
	cmd := m.startAction(action, act, its, false, true)
	m.marks = marks
	what := its[0].ID
	if len(its) > 1 {
		what = fmt.Sprintf("%d items", len(its))
	}
	return startedNotice(cmd, act.Type != "run", fmt.Sprintf("auto-started %s on %s", action, what), "auto: ")
}

// startedNotice rewrites the footer notice of an unattended start cmd: ok for a session start that succeeded,
// otherwise the start's own notice behind prefix.
func startedNotice(cmd tea.Cmd, isSession bool, ok, prefix string) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		d, done := msg.(doneMsg)
		if !done {
			return msg
		}
		if d.ok && isSession {
			d.notice = ok
		} else {
			d.notice = prefix + d.notice
		}
		return d
	}
}

// queuedRun starts a manual queue entry on it unattended: no prompt, no jump, the user's marks kept.
func (m *Model) queuedRun(e autoEntry, act config.Action, it item.Item) tea.Cmd {
	if e.resume {
		return startedNotice(m.resumeItem(it, false), true, fmt.Sprintf("resumed queued %s on %s", e.action, it.ID), "queued: ")
	}
	marks := m.marks
	cmd := m.startAction(e.action, act, []item.Item{it}, false, true)
	m.marks = marks
	return startedNotice(cmd, act.Type != "run", fmt.Sprintf("started queued %s on %s", e.action, it.ID), "queued: ")
}

// drainAuto starts queued entries, oldest first, while max_claude_sessions has room; :auto off holds the on_enter
// entries, manual ones still drain. An entry whose item left its view (on_enter entries), no longer matches the
// action's when, is busy or already runs it is dropped.
func (m *Model) drainAuto() tea.Cmd {
	defer m.refreshQueue()
	var cmds []tea.Cmd
	for i := 0; i < len(m.auto.queue); {
		e := m.auto.queue[i]
		if m.auto.off && !e.manual {
			i++
			continue
		}
		it, ok := m.autoLookup(e)
		act := m.entryAction(e)
		why := ""
		if ok && !e.resume {
			if why = m.dropReason(e.action, act, it); why != "" {
				ok = false
			}
		}
		if ok && m.autoRoom(act, 1) == 0 {
			break
		}
		m.auto.queue = append(m.auto.queue[:i], m.auto.queue[i+1:]...)
		switch {
		case !ok:
			if e.manual && why != "" {
				m.say(lvWarn, fmt.Sprintf("dropped queued %s on %s: %s", e.action, e.id, why))
			}
		case e.manual:
			cmds = append(cmds, m.queuedRun(e, act, it))
		default:
			cmds = append(cmds, m.autoRun(e.action, act, []item.Item{it}))
		}
	}
	return tea.Batch(cmds...)
}

// entryAction is the action a queue entry starts: the configured one, else the one a manual entry was started with.
func (m *Model) entryAction(e autoEntry) config.Action {
	if act, ok := m.cfg.Action[e.action]; ok || !e.manual {
		return act
	}
	return e.act
}

// enqueue queues a manual start of action act on it, unless the item is queued already; it says which.
func (m *Model) enqueue(action string, act config.Action, it item.Item, resume bool) {
	if m.queued(it.ID) != "" {
		return
	}
	prov := m.views[m.cur].cfg.Provider
	if m.onQueue() {
		prov = m.queueProvider(it)
	}
	m.auto.queue = append(m.auto.queue, autoEntry{view: m.views[m.cur].cfg.Name, action: action, id: it.ID,
		manual: true, resume: resume, act: act, it: it, prov: prov})
	m.refreshQueue()
}

// autoLookup finds the item of a queue entry: for a manual entry as its provider lists it now, else as it was
// queued; for an on_enter entry in its view, false when the view is gone, no longer names the entry's action as
// on_enter, or does not list the item.
func (m *Model) autoLookup(e autoEntry) (item.Item, bool) {
	if e.manual {
		for _, it := range m.data[e.prov] {
			if it.ID == e.id {
				return it, true
			}
		}
		return e.it, true
	}
	for _, v := range m.views {
		if v.cfg.Name != e.view || v.cfg.OnEnter != e.action {
			continue
		}
		for _, it := range v.members {
			if it.ID == e.id {
				return it, true
			}
		}
	}
	return item.Item{}, false
}

// queued is the action the item with id waits in the queue for, "" when it is not queued.
func (m *Model) queued(id string) string {
	for _, e := range m.auto.queue {
		if e.id == id {
			return e.action
		}
	}
	return ""
}

// autoCommand handles :auto [on|off].
func (m *Model) autoCommand(arg string) tea.Cmd {
	switch arg {
	case "on":
		m.auto.off = false
		m.say(lvInfo, "auto-runs on")
		return m.drainAuto()
	case "off":
		m.auto.off = true
		m.say(lvInfo, "auto-runs paused")
	case "":
		state := "on"
		if m.auto.off {
			state = "off"
		}
		m.say(lvInfo, fmt.Sprintf("auto-runs %s, %d queued", state, len(m.auto.queue)))
	default:
		m.say(lvWarn, "usage: :auto [on|off]")
	}
	return nil
}
