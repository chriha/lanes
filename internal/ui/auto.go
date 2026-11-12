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
	off   bool                       // :auto off: nothing fires, the queue does not drain
	seen  map[string]map[string]bool // by view name: the item IDs the view listed since the config was applied
	left  map[string]map[string]bool // by view name: seen items a later fetch found gone (on_enter_repeat views)
	queue []autoEntry                // starts held back by max_claude_sessions, oldest first
}

// autoEntry is an on_enter start waiting in the queue for a free session.
type autoEntry struct{ view, action, id string }

// autoFetched runs after a successful full fetch of provider prov: it drops queued items that left their view,
// drains the queue, then fires the on_enter action of each view of prov on its new arrivals.
func (m *Model) autoFetched(prov string) tea.Cmd {
	defer m.refreshQueue()
	keep := m.auto.queue[:0]
	for _, e := range m.auto.queue {
		if _, ok := m.autoLookup(e); ok {
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
	if cmd == nil {
		return nil
	}
	what := its[0].ID
	if len(its) > 1 {
		what = fmt.Sprintf("%d items", len(its))
	}
	isSession := act.Type != "run"
	return func() tea.Msg {
		msg := cmd()
		d, ok := msg.(doneMsg)
		if !ok {
			return msg
		}
		if d.ok && isSession {
			d.notice = fmt.Sprintf("auto-started %s on %s", action, what)
		} else {
			d.notice = "auto: " + d.notice
		}
		return d
	}
}

// drainAuto starts queued entries, oldest first, while :auto is on and max_claude_sessions has room. An entry
// whose item left its view, no longer matches the action's when, is busy or already runs it is dropped.
func (m *Model) drainAuto() tea.Cmd {
	defer m.refreshQueue()
	if m.auto.off {
		return nil
	}
	var cmds []tea.Cmd
	for len(m.auto.queue) > 0 {
		e := m.auto.queue[0]
		it, ok := m.autoLookup(e)
		act := m.cfg.Action[e.action]
		if ok {
			match, _, _ := matchWhen(act, []item.Item{it})
			ok = len(match) == 1 && !m.autoBlocked(e.action, act, it)
		}
		if ok && m.autoRoom(act, 1) == 0 {
			break
		}
		m.auto.queue = m.auto.queue[1:]
		if ok {
			cmds = append(cmds, m.autoRun(e.action, act, []item.Item{it}))
		}
	}
	return tea.Batch(cmds...)
}

// autoLookup finds the item of a queue entry in its view; false when the view is gone, no longer names the
// entry's action as on_enter, or does not list the item.
func (m *Model) autoLookup(e autoEntry) (item.Item, bool) {
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
