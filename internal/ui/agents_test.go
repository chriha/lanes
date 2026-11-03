package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/session"
)

// transcriptLine is one assistant message of the lead, as Claude Code writes it.
const transcriptLine = `{"type":"assistant","timestamp":"2026-11-01T10:00:02Z","message":{"id":"m1","model":"claude-opus-4-1",` +
	`"role":"assistant","content":[{"type":"text","text":"Looking at the ticket."}],"usage":{"input_tokens":10,"output_tokens":20}}}` + "\n"

// withTranscript writes a lead transcript into a temp dir and records it as the Claude session of tmux session
// name, the way the hooks' sidecar does.
func withTranscript(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "claude-1.jsonl")
	if err := os.WriteFile(path, []byte(transcriptLine), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(app.StateDir(), "sessions")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(map[string]string{"session_id": "claude-1", "transcript_path": path})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".claude.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// agentModel is a model at the agent view's full size whose item 1 has the sessions infos, newest first.
func agentModel(t *testing.T, infos ...session.Info) *Model {
	t.Helper()
	m := sessModel(t, infos...)
	m.width, m.height = 90, 46
	m.onSessions(sessionsMsg{infos: infos})
	return m
}

func working(name string) session.Info {
	return session.Info{Name: name, Action: "fix", ItemKey: "1", Claude: "working", Created: time.Now().Add(-time.Minute)}
}

func TestAgentKeyOpensTheView(t *testing.T) {
	tests := []struct {
		name     string
		sessions bool     // press A on the sessions screen instead of the list
		cursor   int      // sessions screen cursor
		claude   []string // sessions with a recorded Claude transcript
		want     string   // session the view opens on, "" for none
		header   string   // what the header names
		notice   string
	}{
		{name: "item: newest Claude session", claude: []string{"fix~1", "review~1"}, want: "fix~1", header: "first"},
		{name: "item: skips sessions without Claude", claude: []string{"review~1"}, want: "review~1", header: "first"},
		{name: "item: no Claude session", notice: "no Claude session for this item"},
		{name: "sessions screen: selected session", sessions: true, cursor: 1, claude: []string{"fix~1", "review~1"}, want: "review~1", header: "review~1"},
		{name: "sessions screen: no Claude session", sessions: true, claude: []string{"review~1"}, notice: "no Claude session in fix~1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := agentModel(t, working("fix~1"), working("review~1"))
			for _, name := range tt.claude {
				withTranscript(t, name)
			}
			back := modeNormal
			if tt.sessions {
				back = modeSessions
				m.mode, m.sessCursor = modeSessions, tt.cursor
			}
			cmd := press(m, "A")
			if tt.want == "" {
				if m.agents != nil || m.mode != back || cmd != nil {
					t.Fatalf("opened a view (mode %v, cmd %v), want none", m.mode, cmd != nil)
				}
				if m.notice != tt.notice {
					t.Errorf("notice = %q, want %q", m.notice, tt.notice)
				}
				return
			}
			if m.agents == nil || m.mode != modeAgents {
				t.Fatalf("mode %v, view %v; want the agent view open", m.mode, m.agents)
			}
			if m.agents.name != tt.want || m.agents.back != back {
				t.Errorf("view of %q back to %v, want %q back to %v", m.agents.name, m.agents.back, tt.want, back)
			}
			if cmd == nil {
				t.Error("opening returned no command: no first read, no tick")
			}
			v := ansi.Strip(m.View())
			if !strings.Contains(v, "AGENT VIEW · "+tt.header) {
				t.Errorf("header does not name %q:\n%s", tt.header, v)
			}
			if !strings.Contains(v, "┃ "+tt.want+" ") {
				t.Errorf("session column does not name %s:\n%s", tt.want, v)
			}
		})
	}
}

func TestAgentViewBackKeys(t *testing.T) {
	for _, back := range []mode{modeNormal, modeSessions} {
		for _, key := range []string{"esc", "q", "A"} {
			m := agentModel(t, working("fix~1"))
			withTranscript(t, "fix~1")
			m.mode = back
			press(m, "A")
			if m.mode != modeAgents {
				t.Fatalf("A did not open the view (mode %v)", m.mode)
			}
			press(m, key)
			if m.mode != back || m.agents != nil {
				t.Errorf("%s from mode %v: mode %v, view open %v; want back, closed", key, back, m.mode, m.agents != nil)
			}
		}
	}
}

func TestAgentViewScrollKeys(t *testing.T) {
	m := agentModel(t, working("fix~1"))
	withTranscript(t, "fix~1")
	m.width, m.height = 90, 14 // a 3-line viewport: the 6 lines of lead and tools scroll 3, and the tools box ends 24 columns past the viewport
	press(m, "A")
	tests := []struct {
		key    string
		wx, wy int
	}{
		{"j", 0, 1}, {"G", 0, 3}, {"k", 0, 2}, {"g", 0, 0}, {"l", 1, 0},
	}
	for _, tt := range tests {
		press(m, tt.key)
		if m.mode != modeAgents || m.agents == nil {
			t.Fatalf("%s closed the agent view", tt.key)
		}
		if x, y := m.agents.fv.Offset(); x != tt.wx || y != tt.wy {
			t.Errorf("after %s: offset %d,%d; want %d,%d", tt.key, x, y, tt.wx, tt.wy)
		}
	}
}

// read runs the open view's read of the transcripts and hands its result to the model, as Bubble Tea would.
func read(t *testing.T, m *Model) (agentSnapMsg, func() bool) {
	t.Helper()
	if m.agents == nil {
		t.Fatal("no agent view open")
	}
	poll := m.agents.poll()
	if poll == nil {
		t.Fatal("the view has no read")
	}
	msg, ok := poll().(agentSnapMsg)
	if !ok {
		t.Fatalf("the read returned %T, want agentSnapMsg", msg)
	}
	_, next := m.Update(msg)
	return msg, func() bool { return next != nil }
}

func TestAgentViewReadsTheTranscript(t *testing.T) {
	m := agentModel(t, working("fix~1"))
	withTranscript(t, "fix~1")
	press(m, "A")
	msg, polls := read(t, m)
	if msg.err != nil || msg.snap.Lead.Model != "claude-opus-4-1" || msg.snap.Lead.LastText != "Looking at the ticket." {
		t.Errorf("read gave %+v, %v; want the lead of the transcript", msg.snap.Lead, msg.err)
	}
	if !polls() {
		t.Error("a live session's read schedules no next read")
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"opus 4.1 · lead", "live"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
}

func TestAgentViewFollowsTheHookState(t *testing.T) {
	m := agentModel(t, working("fix~1"))
	withTranscript(t, "fix~1")
	press(m, "A")
	asking := working("fix~1")
	asking.Claude = "asking"
	m.Update(sessionsMsg{infos: []session.Info{asking}})
	if v := ansi.Strip(m.View()); !strings.Contains(v, "◆ asking") {
		t.Errorf("view does not show the new hook state asking:\n%s", v)
	}
}

func TestAgentViewFreezesWhenTheSessionEnded(t *testing.T) {
	dead := working("fix~1")
	dead.Dead = true
	exited := working("fix~1")
	exited.Claude = "" // SessionEnd removed the hook state: Claude exited, the shell lives on
	tests := []struct {
		name   string
		open   session.Info
		update *session.Info // the session as a later poll of tmux sees it, nil for none
	}{
		{"finished before opening", dead, nil},
		{"Claude exited before opening", exited, nil},
		{"finished while open", working("fix~1"), &dead},
		{"Claude exited while open", working("fix~1"), &exited},
		{"gone while open", working("fix~1"), &session.Info{Name: "other~2", ItemKey: "2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := agentModel(t, tt.open)
			withTranscript(t, "fix~1")
			press(m, "A")
			if m.agents == nil {
				t.Fatal("A did not open the view")
			}
			if tt.update != nil {
				if _, polls := read(t, m); !polls() {
					t.Fatal("a live session's read schedules no next read")
				}
				m.Update(sessionsMsg{infos: []session.Info{*tt.update}})
			}
			msg, polls := read(t, m)
			if msg.snap.Lead.Model == "" {
				t.Error("the last read did not read the transcript")
			}
			if polls() {
				t.Error("an ended session's read schedules another read")
			}
			if v := ansi.Strip(m.View()); !strings.Contains(v, "ended") {
				t.Errorf("frozen view lacks the ended header:\n%s", v)
			}
			if _, cmd := m.Update(agentPollMsg{gen: m.agents.gen}); cmd != nil {
				t.Error("a poll tick of a frozen view starts a read")
			}
			if _, next := m.Update(msg); next != nil {
				t.Error("a read landing after the end schedules another read")
			}
		})
	}
}

func TestClosedAgentViewStopsPolling(t *testing.T) {
	m := agentModel(t, working("fix~1"))
	withTranscript(t, "fix~1")
	press(m, "A")
	if m.agents == nil {
		t.Fatal("A did not open the view")
	}
	gen := m.agents.gen
	poll := m.agents.poll()
	press(m, "esc")
	if _, cmd := m.Update(agentPollMsg{gen: gen}); cmd != nil {
		t.Error("a poll tick of a closed view starts a read")
	}
	if _, cmd := m.Update(poll()); cmd != nil {
		t.Error("a read landing after the view closed schedules another")
	}

	press(m, "A") // reopened: the old view's messages are not the new one's
	if _, cmd := m.Update(agentPollMsg{gen: gen}); cmd != nil {
		t.Error("a poll tick of the closed view starts a read of the reopened one")
	}
	if _, cmd := m.Update(agentPollMsg{gen: m.agents.gen}); cmd == nil {
		t.Error("the open view's poll tick starts no read")
	}
}

func TestAgentKeyIsListed(t *testing.T) {
	if !strings.Contains(builtinKeys, "A") {
		t.Error("A is not in builtinKeys")
	}
	m := sessModel(t)
	m.views[0].items = []item.Item{}
	if help := strings.Join(m.helpLines(), "\n"); !strings.Contains(ansi.Strip(help), "A ") || !strings.Contains(help, "agent view") {
		t.Error("help does not list A")
	}
	m.mode = modeSessions
	if v := ansi.Strip(m.View()); !strings.Contains(v, "A agents") {
		t.Errorf("sessions footer does not hint A:\n%s", v)
	}
}

func TestReadInFlightAtTheEndIsNotTheFinalState(t *testing.T) {
	m := agentModel(t, working("fix~1"))
	path := withTranscript(t, "fix~1")
	press(m, "A")
	if m.agents == nil {
		t.Fatal("A did not open the view")
	}
	inFlight := m.agents.poll() // started while the session was live
	dead := working("fix~1")
	dead.Dead = true
	m.Update(sessionsMsg{infos: []session.Info{dead}})
	_, next := m.Update(inFlight())
	if next == nil {
		t.Fatal("a read started before the end froze the view: what Claude wrote last is never read")
	}
	if v := ansi.Strip(m.View()); strings.Contains(v, "ended") {
		t.Errorf("view froze on a read started before the end:\n%s", v)
	}
	last := strings.Replace(transcriptLine, "Looking at the ticket.", "All done.", 1)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(last); err != nil {
		t.Fatal(err)
	}
	f.Close()
	msg, ok := next().(agentSnapMsg)
	if !ok || msg.snap.Lead.LastText != "All done." {
		t.Fatalf("the read after the end gave %+v, want the last text", msg)
	}
	if _, more := m.Update(msg); more != nil {
		t.Error("the read after the end schedules another read")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "ended") {
		t.Errorf("view did not freeze on the read after the end:\n%s", v)
	}
}

func TestAgentReadErrorIsSaidOnce(t *testing.T) {
	m := agentModel(t, working("fix~1"))
	path := withTranscript(t, "fix~1")
	press(m, "A")
	if m.agents == nil {
		t.Fatal("A did not open the view")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil { // a directory where the transcript was: every read fails
		t.Fatal(err)
	}
	msg, _ := read(t, m)
	if msg.err == nil {
		t.Fatal("reading a directory as the transcript gave no error")
	}
	if !strings.HasPrefix(m.notice, "agent view: ") {
		t.Fatalf("notice = %q, want the read error", m.notice)
	}
	m.say(lvInfo, "")
	read(t, m)
	if m.notice != "" {
		t.Errorf("the same read error is said again: %q", m.notice)
	}
}

func TestAgentViewShowsThe5hShare(t *testing.T) {
	tests := []struct {
		name   string
		before bool // the usage arrives before the view opens
		usage  usage5hMsg
		want   string // the lead's 5h row, spaces collapsed; "" for none
	}{
		{"arrives while open", false, usage5hMsg{pct: 40, ok: true}, "┃ 5h 40% ┃"},
		{"known when the view opens", true, usage5hMsg{pct: 40, ok: true}, "┃ 5h 40% ┃"},
		{"unknown shows no share", false, usage5hMsg{pct: 40}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := agentModel(t, working("fix~1"))
			withTranscript(t, "fix~1")
			if tt.before {
				m.Update(tt.usage)
			}
			press(m, "A")
			if !tt.before {
				m.Update(tt.usage)
			}
			read(t, m) // the lead has all the session's tokens
			v := strings.Join(strings.Fields(ansi.Strip(m.View())), " ")
			if tt.want == "" && strings.Contains(v, "5h") || tt.want != "" && !strings.Contains(v, tt.want) {
				t.Errorf("view = %q, want the 5h row %q", v, tt.want)
			}
		})
	}
}

func TestEndedAgentViewTakesALate5hUsage(t *testing.T) {
	dead := working("fix~1")
	dead.Dead = true
	m := agentModel(t, dead)
	withTranscript(t, "fix~1")
	press(m, "A")
	read(t, m) // the final read freezes the view before any 5h usage came in
	m.Update(usage5hMsg{pct: 40, ok: true})
	if v := strings.Join(strings.Fields(ansi.Strip(m.View())), " "); !strings.Contains(v, "┃ 5h 40% ┃") {
		t.Errorf("frozen view without a 5h figure ignores the first one: %q", v)
	}
	m.Update(usage5hMsg{pct: 55, ok: true})
	if v := strings.Join(strings.Fields(ansi.Strip(m.View())), " "); !strings.Contains(v, "┃ 5h 40% ┃") {
		t.Errorf("frozen view does not keep the figure it has: %q", v)
	}
}

// fakeSampler makes m's agent views sample with a fake that records the pids it samples and returns procs.
func fakeSampler(m *Model, procs []session.Proc) *[]int {
	var pids []int
	m.sampler = func() func(int) []session.Proc {
		return func(pid int) []session.Proc {
			pids = append(pids, pid)
			return procs
		}
	}
	return &pids
}

// sampleOnce runs the open view's sample and hands its result to the model; next tells whether the model
// scheduled another.
func sampleOnce(t *testing.T, m *Model) (msg agentProcsMsg, next func() bool) {
	t.Helper()
	if m.agents == nil {
		t.Fatal("no agent view open")
	}
	msg, ok := m.agents.sampleProcs()().(agentProcsMsg)
	if !ok {
		t.Fatalf("the sample returned %T, want agentProcsMsg", msg)
	}
	_, cmd := m.Update(msg)
	return msg, func() bool { return cmd != nil }
}

func TestAgentViewSamplesTheProcesses(t *testing.T) {
	procs := []session.Proc{
		{PID: 4242, PPID: 1, Command: "-zsh"},
		{PID: 4243, PPID: 4242, CPU: 12, HasCPU: true, RSS: 300 << 20, Command: "claude"},
	}
	open := working("fix~1")
	open.PID = 4242
	m := agentModel(t, open)
	m.width = 300 // the processes box is on screen without scrolling
	pids := fakeSampler(m, procs)
	withTranscript(t, "fix~1")
	press(m, "A")
	if m.agents == nil {
		t.Fatal("A did not open the view")
	}
	msg, next := sampleOnce(t, m)
	if fmt.Sprint(*pids) != "[4242]" || len(msg.procs) != 2 {
		t.Errorf("sampled pids %v giving %d processes, want the pane's pid 4242 once giving 2", *pids, len(msg.procs))
	}
	if !next() {
		t.Error("a sample of a live session schedules no next one")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "processes") || !strings.Contains(v, "300M") {
		t.Errorf("view lacks the processes box with claude's memory:\n%s", v)
	}
	if _, cmd := m.Update(agentProcPollMsg{gen: m.agents.gen}); cmd == nil {
		t.Error("the open view's sample tick starts no sample")
	}
	if _, cmd := m.Update(agentProcPollMsg{gen: m.agents.gen - 1}); cmd != nil {
		t.Error("a sample tick of an older view starts a sample")
	}
}

func TestAgentViewStopsSampling(t *testing.T) {
	procs := []session.Proc{{PID: 4242, PPID: 1, Command: "claude"}}
	live := working("fix~1")
	live.PID = 4242
	dead := live
	dead.Dead = true
	tests := []struct {
		name string
		open session.Info
		stop func(m *Model) // nil: the view never samples
	}{
		{"closed", live, func(m *Model) { press(m, "esc") }},
		{"session finished", live, func(m *Model) { m.Update(sessionsMsg{infos: []session.Info{dead}}) }},
		{"session gone", live, func(m *Model) { m.Update(sessionsMsg{infos: nil}) }},
		{"pane pid unknown", working("fix~1"), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := agentModel(t, tt.open)
			fakeSampler(m, procs)
			withTranscript(t, "fix~1")
			press(m, "A")
			if m.agents == nil {
				t.Fatal("A did not open the view")
			}
			gen := m.agents.gen
			if tt.stop != nil {
				if _, next := sampleOnce(t, m); !next() {
					t.Fatal("a live session's sample schedules no next one")
				}
				landing := m.agents.sampleProcs()
				tt.stop(m)
				if _, cmd := m.Update(landing()); cmd != nil {
					t.Error("a sample landing after the stop schedules another")
				}
			}
			if _, cmd := m.Update(agentProcPollMsg{gen: gen}); cmd != nil {
				t.Error("a sample tick after the stop starts a sample")
			}
		})
	}
}
