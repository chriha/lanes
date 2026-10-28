package ui

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"text/template"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/filter"
	"github.com/chriha/lanes/internal/history"
	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/session"
)

type confirmState struct {
	prompt string
	do     func() tea.Cmd
	back   mode // the screen the prompt returns to
}

type sessionsMsg struct {
	infos []session.Info
	err   error
}
type doneMsg struct {
	notice  string
	refresh bool
	keys    []string // items the action ran on; their busy lock ends here (or after the refetch)
	ok      bool     // every job succeeded
	jump    string   // session to attach to once the start has succeeded
}

// busyState locks an item while an action on it starts or runs, and after a successful refresh run
// until every provider in refetch has landed a fetch that started after the run.
type busyState struct {
	action  string
	refetch map[string]bool
}
type pollNow struct{}

type job struct {
	name, label, itemKey, cwd, cmd string
	restart                        bool
}

func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func (m *Model) pollSessions() tea.Cmd {
	if m.sess == nil || m.polling {
		return nil
	}
	m.polling = true
	mgr := m.sess
	return func() tea.Msg {
		in, err := mgr.List()
		return sessionsMsg{in, err}
	}
}

func (m *Model) onSessions(msg sessionsMsg) {
	m.polling = false
	if msg.err != nil {
		return
	}
	prev := map[string]session.Info{}
	for _, s := range m.sessions {
		prev[s.Name] = s
	}
	m.sessByItem = map[string][]session.Info{}
	for _, s := range msg.infos {
		if p, ok := prev[s.Name]; ok && !m.cfg.Settings.NoNotify {
			m.maybeNotify(p, s)
		}
		if p, ok := prev[s.Name]; ok && !p.Dead && s.Dead {
			ev := "finished"
			if s.Exit != 0 {
				ev = "failed"
			}
			history.Append(history.Event{Item: s.ItemKey, Action: s.Action, Event: ev, Exit: s.Exit})
		}
		m.sessByItem[s.ItemKey] = append(m.sessByItem[s.ItemKey], s)
	}
	sel := m.selectedSession()
	m.sessions = msg.infos
	m.restoreSessCursor(sel)
}

func (m *Model) sessionsOf(it item.Item) []session.Info { return m.sessByItem[session.Key(it.ID)] }

// targets are the marked items of the current view, or the item under the cursor.
func (m *Model) targets() []item.Item {
	var out []item.Item
	for _, it := range m.items() {
		if m.marks[it.ID] {
			out = append(out, it)
		}
	}
	if len(out) == 0 && m.cursor < len(m.items()) {
		out = append(out, m.items()[m.cursor])
	}
	return out
}

// matchWhen splits its into the items the action's when expression matches and the ones it skips.
// An expression that fails on an item counts as no match; the first such error is returned.
func matchWhen(act config.Action, its []item.Item) (match, skip []item.Item, err error) {
	f, err := filter.Compile(act.When) // validated at config load; an error here skips everything
	if err != nil {
		return nil, its, err
	}
	for _, it := range its {
		ok, merr := f.Match(it.Env())
		if merr != nil && err == nil {
			err = merr
		}
		if ok && merr == nil {
			match = append(match, it)
		} else {
			skip = append(skip, it)
		}
	}
	return match, skip, err
}

// whenNotice reports items an action skipped because its when expression did not match them.
func whenNotice(skipped int, err error) string {
	if skipped == 0 {
		return ""
	}
	if err != nil {
		return fmt.Sprintf("skipped %d item(s) (when: %v)", skipped, err)
	}
	return fmt.Sprintf("skipped %d item(s) (when)", skipped)
}

func ids(its []item.Item) string {
	s := make([]string, len(its))
	for i, it := range its {
		s[i] = it.ID
	}
	return strings.Join(s, ", ")
}

func (m *Model) ask(prompt string, do func() tea.Cmd) {
	m.confirm = &confirmState{prompt, do, m.mode}
	m.mode = modeConfirm
}

// ParseTemplate parses an action's run or cwd template.
func ParseTemplate(text string) (*template.Template, error) {
	return template.New("").Funcs(template.FuncMap{"q": shq}).Option("missingkey=error").Parse(text)
}

func render(text string, data map[string]any) (string, error) {
	t, err := ParseTemplate(text)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	if err := t.Execute(&b, data); err != nil {
		if m := missingKey.FindStringSubmatch(err.Error()); m != nil {
			return "", fmt.Errorf("no field %q; this item's identifier is %q (see fields with p)", m[1], identifierKey(data))
		}
		return "", err
	}
	return b.String(), nil
}

// missingKey matches text/template's error for a top-level key the data does not have (not .vars.x and the like).
// It relies on the error text; the tests catch a change in it.
var missingKey = regexp.MustCompile(`at <\.([^.>\s]+)>: map has no entry for key "`)

// identifierKey is the field that identifies an item: iid (gitlab), number (github), key (linear), else id.
func identifierKey(data map[string]any) string {
	for _, k := range []string{"iid", "number", "key"} {
		if _, ok := data[k]; ok {
			return k
		}
	}
	return "id"
}

func (m *Model) runAction(name string, act config.Action) tea.Cmd {
	if m.sess == nil {
		m.notice = "tmux unavailable"
		return nil
	}
	all, unmatched, werr := matchWhen(act, m.targets())
	skippedWhen := whenNotice(len(unmatched), werr)
	if len(all) == 0 {
		if skippedWhen != "" {
			m.notice = name + ": " + skippedWhen
		}
		return nil
	}
	// a busy item is starting or running an action: skip it, whatever the action
	var ts []item.Item
	var busy []string
	for _, it := range all {
		if b, ok := m.busy[session.Key(it.ID)]; ok {
			busy = append(busy, fmt.Sprintf("%s busy (%s)", it.ID, b.action))
			continue
		}
		ts = append(ts, it)
	}
	if len(ts) == 0 {
		m.notice = withNote(strings.Join(busy, ", "), skippedWhen)
		return nil
	}
	var jobs []job
	skipped := 0
	headless := act.Type == "run"
	jump := act.Jump && !headless && len(all) == 1 // several targets start all and jump into none
	vars := act.Vars
	if vars == nil {
		vars = map[string]any{}
	}
	for _, it := range ts {
		sname := session.Name(name, it.ID)
		restart := false
		for _, s := range m.sessionsOf(it) {
			if headless {
				break
			}
			if s.Name != sname {
				continue
			}
			if !s.Dead {
				if jump {
					return m.jumpTo(sname)
				}
				skipped++
				goto next
			}
			restart = true
		}
		{
			data := it.Env()
			data["settings"], data["session"], data["action"], data["vars"] = m.settings, sname, name, vars
			cmd, err := render(act.Run, data)
			if err != nil {
				m.notice = fmt.Sprintf("action %s: %v", name, err)
				return nil
			}
			cwd, err := render(act.Cwd, data)
			if err != nil {
				m.notice = fmt.Sprintf("action %s cwd: %v", name, err)
				return nil
			}
			jobs = append(jobs, job{sname, it.ID, session.Key(it.ID), cwd, cmd, restart})
		}
	next:
	}
	if len(jobs) == 0 {
		m.notice = fmt.Sprintf("%s already running", name)
		if len(busy) > 0 {
			m.notice += ", " + strings.Join(busy, ", ")
		}
		m.notice = withNote(m.notice, skippedWhen)
		return nil
	}
	start := func() tea.Cmd {
		m.lock(name, jobs)
		var cmd tea.Cmd
		if headless {
			cmd = m.executeRun(name, jobs, act.Refresh, len(busy), skippedWhen)
		} else {
			attach := ""
			if jump {
				attach = jobs[0].name
			}
			cmd = m.execute(name, jobs, skipped, len(busy), skippedWhen, attach)
		}
		for _, it := range unmatched {
			m.marks[it.ID] = true // skipped items stay marked, so it shows what did not run
		}
		return cmd
	}
	if len(all) > 1 || act.Confirm == "single" {
		what := ts[0].ID
		if len(ts) > 1 {
			what = fmt.Sprintf("%d items (%s)", len(ts), ids(ts))
		}
		m.ask(fmt.Sprintf("Run %q on %s?", name, what), start)
		return nil
	}
	return start()
}

// lock marks the jobs' items busy with action and says so in the footer; doneMsg releases them.
func (m *Model) lock(action string, jobs []job) {
	labels := make([]string, len(jobs))
	for i, j := range jobs {
		m.busy[j.itemKey] = busyState{action: action}
		labels[i] = j.label
	}
	m.notice = fmt.Sprintf("starting %s on %s…", action, strings.Join(labels, ", "))
}

func jobKeys(jobs []job) []string {
	keys := make([]string, len(jobs))
	for i, j := range jobs {
		keys[i] = j.itemKey
	}
	return keys
}

// withNote appends note to s, comma-separated, when note is set.
func withNote(s, note string) string {
	if note == "" {
		return s
	}
	if s == "" {
		return note
	}
	return s + ", " + note
}

// execute starts the jobs' sessions; with attach set, the doneMsg asks to attach to it once all started.
func (m *Model) execute(action string, jobs []job, skipped, busy int, skippedWhen, attach string) tea.Cmd {
	m.marks = map[string]bool{}
	mgr := m.sess
	keys := jobKeys(jobs)
	return func() tea.Msg {
		var errs []string
		started := 0
		for _, j := range jobs {
			if j.restart {
				_ = mgr.Kill(j.name)
			}
			if err := mgr.Start(j.name, j.cwd, j.cmd); err != nil {
				errs = append(errs, err.Error())
				continue
			}
			started++
			history.Append(history.Event{Item: j.itemKey, Action: action, Event: "started"})
		}
		note := fmt.Sprintf("started %s on %d item(s)", action, started)
		if skipped > 0 {
			note += fmt.Sprintf(", %d already running", skipped)
		}
		if busy > 0 {
			note += fmt.Sprintf(", %d busy", busy)
		}
		note = withNote(note, skippedWhen)
		if len(errs) > 0 {
			note += ": " + errs[0]
		}
		d := doneMsg{notice: note, keys: keys, ok: len(errs) == 0}
		if d.ok {
			d.jump = attach
		}
		return d
	}
}

// executeRun runs the jobs headless and in parallel; the footer reports the outcome.
func (m *Model) executeRun(action string, jobs []job, refresh bool, busy int, skippedWhen string) tea.Cmd {
	m.marks = map[string]bool{}
	keys := jobKeys(jobs)
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return func() tea.Msg {
		fails := make([]string, len(jobs))
		var wg sync.WaitGroup
		for i, j := range jobs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				history.Append(history.Event{Item: j.itemKey, Action: action, Event: "started"})
				cmd := exec.Command(shell, "-c", j.cmd)
				cmd.Dir = j.cwd
				out, err := cmd.CombinedOutput()
				if err == nil {
					history.Append(history.Event{Item: j.itemKey, Action: action, Event: "finished"})
					return
				}
				code := -1
				var ee *exec.ExitError
				if errors.As(err, &ee) {
					code = ee.ExitCode()
				}
				history.Append(history.Event{Item: j.itemKey, Action: action, Event: "failed", Exit: code})
				fails[i] = j.label + ": " + lastLine(string(out), err)
			}()
		}
		wg.Wait()
		var bad []string
		for _, f := range fails {
			if f != "" {
				bad = append(bad, f)
			}
		}
		note := fmt.Sprintf("✓ %s done on %d item(s)", action, len(jobs))
		if busy > 0 {
			note += fmt.Sprintf(", %d busy", busy)
		}
		note = withNote(note, skippedWhen)
		if len(bad) > 0 {
			note = fmt.Sprintf("✗ %s failed on %d/%d — %s", action, len(bad), len(jobs), bad[0])
		}
		return doneMsg{notice: note, refresh: refresh, keys: keys, ok: len(bad) == 0}
	}
}

func lastLine(out string, err error) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if l := strings.TrimSpace(lines[len(lines)-1]); l != "" {
		return l
	}
	return err.Error()
}

func (m *Model) killTargets() tea.Cmd {
	var infos []session.Info
	for _, it := range m.targets() {
		infos = append(infos, m.sessionsOf(it)...)
	}
	if len(infos) == 0 {
		m.notice = "no sessions here"
		return nil
	}
	return m.killSessions(infos)
}

// killSessions closes infos: dead ones right away, after a y/n prompt when any is still running.
func (m *Model) killSessions(infos []session.Info) tea.Cmd {
	alive := 0
	for _, s := range infos {
		if !s.Dead {
			alive++
		}
	}
	mgr := m.sess
	kill := func() tea.Cmd {
		return func() tea.Msg {
			for _, s := range infos {
				if !s.Dead {
					history.Append(history.Event{Item: s.ItemKey, Action: s.Action, Event: "killed"})
				}
				_ = mgr.Kill(s.Name)
			}
			return doneMsg{notice: fmt.Sprintf("closed %d session(s)", len(infos))}
		}
	}
	if alive == 0 {
		return kill()
	}
	m.ask(fmt.Sprintf("Kill %d running session(s)?", alive), kill)
	return nil
}

// jump attaches to the newest session of the item under the cursor.
func (m *Model) jump(it item.Item) tea.Cmd {
	ss := m.sessionsOf(it)
	if len(ss) == 0 {
		return nil
	}
	return m.jumpTo(ss[0].Name)
}

// jumpTo attaches to the session called name; sessions are re-polled on return.
func (m *Model) jumpTo(name string) tea.Cmd {
	cmd, popup := m.sess.JumpCmd(name)
	if popup {
		return func() tea.Msg {
			_ = cmd.Run()
			return pollNow{}
		}
	}
	return tea.ExecProcess(cmd, func(error) tea.Msg { return pollNow{} })
}

func (m *Model) showHistory() {
	it := m.items()
	if m.cursor >= len(it) {
		return
	}
	cur := it[m.cursor]
	m.histTitle = cur.ID + "  " + cur.Title
	m.hist = nil
	evs := history.For(session.Key(cur.ID))
	if len(evs) > 25 {
		evs = evs[len(evs)-25:]
	}
	for _, e := range evs {
		line := fmt.Sprintf("%s  %-9s %s", e.Time.Format("2006-01-02 15:04"), e.Event, e.Action)
		if e.Event == "failed" {
			line += fmt.Sprintf(" (exit %d)", e.Exit)
		}
		m.hist = append(m.hist, line)
	}
	if len(m.hist) == 0 {
		m.hist = []string{"no history yet"}
	}
	m.mode = modeHistory
}

func (m *Model) keyConfirm(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	c := m.confirm
	m.mode, m.confirm = modeNormal, nil
	if c != nil {
		m.mode = c.back
	}
	if s := k.String(); c != nil && (s == "y" || s == "enter") {
		return m, c.do()
	}
	m.notice = "cancelled"
	return m, nil
}

func (m *Model) historyView() string {
	var b strings.Builder
	b.WriteString(sTitle.Render(" history — "+m.histTitle) + "\n\n")
	for _, l := range m.hist {
		b.WriteString("  " + sText.Render(l) + "\n")
	}
	b.WriteString("\n" + sDim.Render("  any key to close"))
	return b.String()
}

func (m *Model) itemLabel(key string) string {
	for _, items := range m.data {
		for _, it := range items {
			if session.Key(it.ID) == key {
				return it.ID + " " + it.Title
			}
		}
	}
	return key
}

func (m *Model) maybeNotify(prev, cur session.Info) {
	switch {
	case !prev.Dead && cur.Dead && cur.Exit == 0:
		notify(app.Name+": "+cur.Action+" finished", m.itemLabel(cur.ItemKey))
	case !prev.Dead && cur.Dead:
		notify(fmt.Sprintf("%s: %s failed (%d)", app.Name, cur.Action, cur.Exit), m.itemLabel(cur.ItemKey))
	case prev.Claude != cur.Claude && cur.Claude == "asking":
		notify(app.Name+": "+cur.Action+" has a question", m.itemLabel(cur.ItemKey))
	case prev.Claude != cur.Claude && cur.Claude == "waiting":
		notify(app.Name+": "+cur.Action+" is waiting for you", m.itemLabel(cur.ItemKey))
	}
}

// sessionSummary is the header's at-a-glance count of live sessions that need attention.
func (m *Model) sessionSummary() string {
	run, ask, wait := session.Count(m.sessions)
	var parts []string
	if run > 0 {
		parts = append(parts, badgeStyle("info").Render(fmt.Sprintf("◐ %d", run)))
	}
	if ask > 0 {
		parts = append(parts, badgeStyle("warn").Render(fmt.Sprintf("? %d", ask)))
	}
	if wait > 0 {
		parts = append(parts, badgeStyle("warn").Render(fmt.Sprintf("● %d", wait)))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ") + "   "
}
