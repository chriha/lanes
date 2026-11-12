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
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/config"
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
	level   level
	refresh bool
	keys    []string // items the action ran on; their busy lock ends here (or after the refetch)
	ok      bool     // every job succeeded
	jump    string   // session to attach to once the start has succeeded
	action  string   // run actions: the action that ran, for its on_success / on_failure
	exits   []int    // run actions: exit code per item, in the order of keys
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

// onSessions takes in a poll; it returns the history writes of the sessions seen dying, then their follow-ups' starts.
func (m *Model) onSessions(msg sessionsMsg) tea.Cmd {
	m.polling = false
	if msg.err != nil {
		return nil
	}
	var ended []session.Info // chained once sessByItem is complete, so the follow-up sees every live session
	var writes []func()      // end events, written off the UI goroutine: they read the Claude transcripts
	prev := map[string]session.Info{}
	for _, s := range m.sessions {
		prev[s.Name] = s
	}
	m.sessByItem = map[string][]session.Info{}
	alive := map[string]bool{}
	for _, s := range msg.infos {
		alive[s.Name] = !s.Dead
		if p, ok := prev[s.Name]; ok && !m.cfg.Settings.NoNotify {
			m.maybeNotify(p, s)
		}
		if p, ok := prev[s.Name]; ok && !p.Dead && s.Dead {
			ev := "finished"
			if s.Exit != 0 {
				ev = "failed"
			}
			writes = append(writes, eventWriter(s, ev, s.Exit))
			if !m.interrupted[s.Name] { // killed sessions vanish instead of dying, so they never get here
				ended = append(ended, s)
			}
		}
		m.sessByItem[s.ItemKey] = append(m.sessByItem[s.ItemKey], s)
	}
	for name := range m.interrupted {
		if !alive[name] {
			delete(m.interrupted, name) // died or gone: a restart under the same name starts with SIGTERM again
		}
	}
	sel := m.selectedSession()
	m.sessions = msg.infos
	m.restoreSessCursor(sel)
	var chains []tea.Cmd
	for _, s := range ended {
		chains = append(chains, m.chain(s.Action, s.ItemKey, s.Exit))
	}
	if len(writes) == 0 {
		return tea.Batch(chains...)
	}
	record := func() tea.Msg {
		for _, w := range writes {
			w()
		}
		return nil
	}
	return tea.Sequence(record, tea.Batch(chains...)) // an end event goes into the history before its follow-up's start
}

// chain starts, on the item with key, the follow-up of action for exit (0 → on_success, else on_failure) through
// the normal start path, and says so in the footer. Marks are kept and the follow-up never jumps.
func (m *Model) chain(action, key string, exit int) tea.Cmd {
	next, ev := followUp(m.cfg.Action[action], exit)
	if next == "" {
		return nil
	}
	var target *item.Item
	for _, items := range m.data {
		for i := range items {
			if session.Key(items[i].ID) == key {
				target = &items[i]
			}
		}
	}
	if target == nil {
		m.say(lvWarn, fmt.Sprintf("%s %s on %s → %s: %s no longer listed", action, ev, key, next, key))
		return nil
	}
	prefix := fmt.Sprintf("%s %s on %s → ", action, ev, target.ID)
	nact := m.cfg.Action[next]
	nact.Jump = false // lanes never pulls the user into a session out of the blue
	marks := m.marks
	// no limit check: no one is there to answer its prompt
	cmd := m.startAction(next, nact, []item.Item{*target}, false, false)
	m.marks = marks // the follow-up runs on its item alone; the user's marks stay
	if cmd != nil {
		m.notice = prefix + "starting " + next
	} else {
		m.notice = prefix + next + ": " + strings.TrimPrefix(m.notice, next+": ")
	}
	return cmd
}

// followUp names the action act chains to for exit (0 → on_success, else on_failure) and the outcome's history event.
func followUp(act config.Action, exit int) (next, ev string) {
	if exit != 0 {
		return act.OnFailure, "failed"
	}
	return act.OnSuccess, "finished"
}

// clock is the time source for history durations; tests replace it.
var clock = time.Now

// recordEvent appends an event of session s to the history; see eventWriter. It reads the transcript: call it off
// the UI goroutine.
func recordEvent(s session.Info, event string, exit int) { eventWriter(s, event, exit)() }

// eventWriter takes, now, what an event of session s carries and may not outlast the moment: the time since the
// session's creation, when that is known, and the ids of the Claude session the hooks saw in it. The returned func
// adds the tokens of that Claude session since the creation, read from its transcript, and appends the event; it
// reads a file of any size, so it runs in a tea.Cmd.
func eventWriter(s session.Info, event string, exit int) func() {
	var dur int64
	if !s.Created.IsZero() {
		dur = seconds(clock().Sub(s.Created))
	}
	e := history.Event{Item: s.ItemKey, Action: s.Action, Event: event, Exit: exit, Dur: dur, ClaudeSession: session.ClaudeSession(s.Name)}
	transcript := session.ClaudeTranscript(s.Name)
	return func() {
		if t, ok := session.TranscriptUsage(transcript, s.Created); ok && t != (session.Tokens{}) {
			e.Tokens = &history.Tokens{Input: t.Input, CacheWrite: t.CacheWrite, CacheRead: t.CacheRead, Output: t.Output}
		}
		history.Append(e)
	}
}

// seconds rounds d to whole seconds.
func seconds(d time.Duration) int64 { return int64(d.Round(time.Second) / time.Second) }

// recordEnd records the end of session s. Its Claude session ids stay (the agent view needs the transcript of a
// finished session); Manager.Kill and Start drop them.
func recordEnd(s session.Info, event string, exit int) { recordEvent(s, event, exit) }

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
	f := act.WhenFilter // compiled once at config load
	if f == nil {
		if act.When != "" { // never compiled: fail closed rather than run on items the expression would exclude
			return nil, its, fmt.Errorf("when %q was not compiled", act.When)
		}
		return its, nil, nil
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

// ParseTemplate parses an action's run template; every value it inserts is shell-escaped (see shellesc.go).
func ParseTemplate(text string) (*template.Template, error) {
	t, err := ParseCwdTemplate(text)
	if err != nil {
		return nil, err
	}
	return t, escapeShell(t)
}

// ParseCwdTemplate parses an action's cwd template: a directory, not shell code, so values go in unchanged.
func ParseCwdTemplate(text string) (*template.Template, error) {
	return template.New("").Funcs(templateFuncs).Option("missingkey=error").Parse(text)
}

// render renders a run template.
func render(text string, data map[string]any) (string, error) {
	t, err := ParseTemplate(text)
	return execTemplate(t, err, data)
}

func renderCwd(text string, data map[string]any) (string, error) {
	t, err := ParseCwdTemplate(text)
	return execTemplate(t, err, data)
}

func execTemplate(t *template.Template, err error, data map[string]any) (string, error) {
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
	if m.onQueue() {
		m.say(lvWarn, "the queue view runs no actions: switch to a view, or drop entries with x")
		return nil
	}
	return m.runActionOn(name, act, m.targets())
}

// runActionOn starts action name on targets: when, busy, queued and already-running items are skipped with a
// notice. Over the session limit it starts what fits and queues the rest.
func (m *Model) runActionOn(name string, act config.Action, targets []item.Item) tea.Cmd {
	return m.startAction(name, act, targets, true, false)
}

// startAction is runActionOn; limit false skips the session limit check and the queue (chain follow-ups and queued
// starts). auto (on_enter and queued starts) never prompts and never jumps.
func (m *Model) startAction(name string, act config.Action, targets []item.Item, limit, auto bool) tea.Cmd {
	if m.sess == nil {
		m.say(lvWarn, "tmux unavailable")
		return nil
	}
	all, unmatched, werr := matchWhen(act, targets)
	skippedWhen := whenNotice(len(unmatched), werr)
	if len(all) == 0 {
		if skippedWhen != "" {
			m.say(lvWarn, name+": "+skippedWhen)
		}
		return nil
	}
	// a busy item is starting or running an action: skip it, whatever the action
	headless := act.Type == "run"
	gate := limit && !headless // over max_claude_sessions the start queues
	var ts []item.Item
	var busy []string
	var busyItems []item.Item
	for _, it := range all {
		if b, ok := m.busy[session.Key(it.ID)]; ok {
			busy = append(busy, fmt.Sprintf("%s busy (%s)", it.ID, b.action))
			busyItems = append(busyItems, it)
			continue
		}
		if gate && m.queued(it.ID) != "" { // a queued item waits for its start already
			busy = append(busy, it.ID+" already queued")
			busyItems = append(busyItems, it)
			continue
		}
		ts = append(ts, it)
	}
	if len(ts) == 0 {
		m.say(lvWarn, withNote(strings.Join(busy, ", "), skippedWhen))
		return nil
	}
	var jobs []job
	skipped := 0
	var runningItems []item.Item
	var jobItems []item.Item
	jump := act.Jump && !headless && !auto && len(all) == 1 // several targets start all and jump into none
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
					m.marks = map[string]bool{} // like execute(): a jump consumes the marks
					return m.jumpTo(sname)
				}
				skipped++
				runningItems = append(runningItems, it)
				goto next
			}
			restart = true
		}
		{
			data := it.Env()
			data["settings"], data["session"], data["action"], data["vars"] = m.settings, sname, name, vars
			cmd, err := render(act.Run, data)
			if err != nil {
				m.say(lvErr, fmt.Sprintf("action %s: %v", name, err))
				return nil
			}
			cwd, err := renderCwd(act.Cwd, data)
			if err != nil {
				m.say(lvErr, fmt.Sprintf("action %s cwd: %v", name, err))
				return nil
			}
			jobs = append(jobs, job{sname, it.ID, session.Key(it.ID), cwd, cmd, restart})
			jobItems = append(jobItems, it)
		}
	next:
	}
	if len(jobs) == 0 {
		note := fmt.Sprintf("%s already running", name)
		if len(busy) > 0 {
			note += ", " + strings.Join(busy, ", ")
		}
		m.say(lvWarn, withNote(note, skippedWhen))
		return nil
	}
	start := func() tea.Cmd {
		run, note := jobs, skippedWhen
		if over := m.overLimit(len(jobs)); gate && over != "" { // start what fits, queue the rest in item order
			n := m.autoRoom(act, len(jobs))
			for _, it := range jobItems[n:] {
				m.enqueue(name, act, it, false)
			}
			run = jobs[:n]
			note = withNote(fmt.Sprintf("%d queued (%s)", len(jobs)-n, over), skippedWhen)
			if n == 0 {
				note = withNote(fmt.Sprintf("queued %s on %s (%s)", name, labelsOf(jobs), over), withNote(strings.Join(busy, ", "), skippedWhen))
			}
		}
		var cmd tea.Cmd
		switch {
		case len(run) == 0:
			m.marks = map[string]bool{}
			m.say(lvWarn, note)
		case headless:
			m.lock(name, run)
			cmd = m.executeRun(name, run, act.Refresh, len(busy), note)
		default:
			m.lock(name, run)
			attach := ""
			if jump && len(run) == len(jobs) {
				attach = run[0].name
			}
			cmd = m.execute(name, run, skipped, len(busy), note, attach)
		}
		for _, it := range append(append(unmatched, busyItems...), runningItems...) {
			m.marks[it.ID] = true // skipped items stay marked, so it shows what did not run
		}
		return cmd
	}
	if !auto && (len(all) > 1 || act.Confirm == "single") {
		what := ts[0].ID
		if len(ts) > 1 {
			what = fmt.Sprintf("%d items (%s)", len(ts), ids(ts))
		}
		m.ask(fmt.Sprintf("Run %q on %s?", name, what), start)
		return nil
	}
	return start()
}

// labelsOf lists the jobs' item labels, comma-separated.
func labelsOf(jobs []job) string {
	labels := make([]string, len(jobs))
	for i, j := range jobs {
		labels[i] = j.label
	}
	return strings.Join(labels, ", ")
}

// overLimit says, when starting n more sessions would take the live action sessions above max_claude_sessions,
// "Limit of N reached (K live)"; "" when within the limit or there is none (0).
func (m *Model) overLimit(n int) string {
	limit := m.cfg.Settings.SessionLimit()
	if limit == 0 {
		return ""
	}
	live := m.liveSessions()
	if live+n <= limit {
		return ""
	}
	return fmt.Sprintf("Limit of %d reached (%d live)", limit, live)
}

// liveSessions counts the action sessions that are alive in the last poll or starting right now (busy with a
// session action, which the poll may not show yet). Dead sessions and headless runs do not count.
func (m *Model) liveSessions() int {
	names := map[string]bool{}
	for _, s := range m.sessions {
		if !s.Dead {
			names[s.Name] = true
		}
	}
	for key, b := range m.busy {
		if a, ok := m.cfg.Action[b.action]; !ok || a.Type != "run" { // not configured: the built-in shell
			names[session.Name(b.action, key)] = true
		}
	}
	return len(names)
}

// resume starts, in its action's session, the resume template of the action whose Claude session ended last on it
// (the newest history event with a claude_session whose action has resume). A dead session is restarted; a
// running one is left alone. Marks are ignored and kept. Over max_claude_sessions the resume is queued.
func (m *Model) resume(it item.Item) tea.Cmd { return m.resumeItem(it, true) }

// resumeItem is resume; limit false skips the session limit and the queue and never jumps (a queued resume).
func (m *Model) resumeItem(it item.Item, limit bool) tea.Cmd {
	if m.sess == nil {
		m.say(lvWarn, "tmux unavailable")
		return nil
	}
	key := session.Key(it.ID)
	if b, ok := m.busy[key]; ok {
		m.say(lvWarn, fmt.Sprintf("%s busy (%s)", it.ID, b.action))
		return nil
	}
	if limit && m.queued(it.ID) != "" {
		m.say(lvWarn, it.ID+" already queued")
		return nil
	}
	var ev *history.Event
	evs := history.For(key)
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].ClaudeSession != "" && m.cfg.Action[evs[i].Action].Resume != "" {
			ev = &evs[i]
			break
		}
	}
	if ev == nil {
		m.say(lvWarn, "nothing to resume for "+it.ID)
		return nil
	}
	name, act := ev.Action, m.cfg.Action[ev.Action]
	sname := session.Name(name, it.ID)
	restart := false
	for _, s := range m.sessionsOf(it) {
		if s.Name != sname {
			continue
		}
		if !s.Dead {
			m.say(lvWarn, "work is still running — enter jumps")
			return nil
		}
		restart = true
	}
	vars := act.Vars
	if vars == nil {
		vars = map[string]any{}
	}
	data := it.Env()
	data["settings"], data["session"], data["action"], data["vars"] = m.settings, sname, name, vars
	data[config.ClaudeSessionField] = ev.ClaudeSession
	cmd, err := render(act.Resume, data)
	if err != nil {
		m.say(lvErr, fmt.Sprintf("action %s resume: %v", name, err))
		return nil
	}
	cwd, err := renderCwd(act.Cwd, data)
	if err != nil {
		m.say(lvErr, fmt.Sprintf("action %s cwd: %v", name, err))
		return nil
	}
	jobs := []job{{sname, it.ID, key, cwd, cmd, restart}}
	attach := ""
	if act.Jump && limit {
		attach = sname
	}
	if over := m.overLimit(len(jobs)); limit && over != "" {
		m.enqueue(name, act, it, true)
		m.say(lvWarn, fmt.Sprintf("queued resume of %s on %s (%s)", name, it.ID, over))
		return nil
	}
	marks := m.marks
	m.lock(name, jobs)
	start := m.execute(name, jobs, 0, 0, "", attach)
	m.marks = marks // execute consumes the marks; resume runs on the cursor item only
	return start
}

// lock marks the jobs' items busy with action and says so in the footer; doneMsg releases them.
func (m *Model) lock(action string, jobs []job) {
	for _, j := range jobs {
		m.busy[j.itemKey] = busyState{action: action}
	}
	m.say(lvInfo, fmt.Sprintf("starting %s on %s…", action, labelsOf(jobs)))
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
		d := doneMsg{notice: note, level: lvInfo, keys: keys, ok: len(errs) == 0}
		if skipped > 0 || busy > 0 || skippedWhen != "" {
			d.level = lvWarn
		}
		if !d.ok {
			d.level = lvErr
		}
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
		exits := make([]int, len(jobs))
		var wg sync.WaitGroup
		for i, j := range jobs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				history.Append(history.Event{Item: j.itemKey, Action: action, Event: "started"})
				cmd := exec.Command(shell, "-c", j.cmd)
				cmd.Dir = j.cwd
				start := clock()
				out, err := cmd.CombinedOutput()
				dur := seconds(clock().Sub(start))
				if err == nil {
					history.Append(history.Event{Item: j.itemKey, Action: action, Event: "finished", Dur: dur})
					return
				}
				code := -1
				var ee *exec.ExitError
				if errors.As(err, &ee) {
					code = ee.ExitCode()
				}
				history.Append(history.Event{Item: j.itemKey, Action: action, Event: "failed", Exit: code, Dur: dur})
				exits[i] = code
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
		if len(bad) > 0 {
			note = fmt.Sprintf("✗ %s failed on %d/%d — %s", action, len(bad), len(jobs), bad[0])
		}
		if busy > 0 {
			note += fmt.Sprintf(", %d busy", busy)
		}
		note = withNote(note, skippedWhen)
		lv := lvInfo
		if busy > 0 || skippedWhen != "" {
			lv = lvWarn
		}
		if len(bad) > 0 {
			lv = lvErr
		}
		return doneMsg{notice: note, level: lv, refresh: refresh, keys: keys, ok: len(bad) == 0, action: action, exits: exits}
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
		m.say(lvWarn, "no sessions here")
		return nil
	}
	return m.killSessions(infos)
}

// killSessions closes infos after a y/n prompt: finished sessions still hold their output.
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
					recordEnd(s, "killed", 0)
				}
				_ = mgr.Kill(s.Name)
			}
			return doneMsg{notice: fmt.Sprintf("closed %d session(s)", len(infos))}
		}
	}
	prompt := fmt.Sprintf("Close %d session(s)?", len(infos))
	if alive > 0 {
		prompt = fmt.Sprintf("Close %d session(s) (%d running)?", len(infos), alive)
	}
	m.ask(prompt, kill)
	return nil
}

// interruptTargets interrupts the running sessions of the marked items / the cursor item (the targets of x).
func (m *Model) interruptTargets() tea.Cmd {
	var infos []session.Info
	for _, it := range m.targets() {
		infos = append(infos, m.sessionsOf(it)...)
	}
	return m.interruptSessions(infos)
}

// interruptSessions stops the commands of the running sessions in infos after a y/n prompt and keeps the
// sessions with their output; dead ones are skipped. A session interrupted before in this run gets SIGKILL.
func (m *Model) interruptSessions(infos []session.Info) tea.Cmd {
	var alive []session.Info
	for _, s := range infos {
		if !s.Dead {
			alive = append(alive, s)
		}
	}
	if len(alive) == 0 {
		m.say(lvWarn, "no running sessions here")
		return nil
	}
	mgr := m.sess
	m.ask(fmt.Sprintf("Interrupt %d running session(s)?", len(alive)), func() tea.Cmd {
		force := map[string]bool{}
		for _, s := range alive {
			force[s.Name] = m.interrupted[s.Name]
		}
		return func() tea.Msg {
			var done []string
			var first error
			for _, s := range alive {
				sig, err := mgr.Interrupt(s.Name, force[s.Name])
				if err != nil && first == nil {
					first = fmt.Errorf("%s: %w", s.Name, err)
				}
				if sig {
					done = append(done, s.Name)
					recordEvent(s, "interrupted", 0) // the session may keep running: keep its Claude session id for the real end
				}
			}
			note, lv := fmt.Sprintf("interrupted %d session(s)", len(done)), lvInfo
			if first != nil {
				note, lv = "✗ interrupt "+first.Error(), lvErr
			}
			return interruptedMsg{names: done, notice: note, level: lv}
		}
	})
	return nil
}

// interruptedMsg reports the sessions an interrupt signalled; the next X on one still alive sends SIGKILL.
type interruptedMsg struct {
	names  []string
	notice string
	level  level
}

// shell runs the built-in shell action on it alone (marks are ignored and kept) and jumps into it.
func (m *Model) shell(it item.Item) tea.Cmd {
	marks := m.marks
	m.marks = map[string]bool{it.ID: true} // ponytail: runAction targets the marks; borrow them for this item
	cmd := m.runAction(config.ShellAction, config.Action{Run: `exec "${SHELL:-/bin/sh}"`, Jump: true})
	m.marks = marks
	return cmd
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
	m.say(lvInfo, "cancelled")
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
