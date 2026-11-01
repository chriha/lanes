package ui

import (
	"encoding/json"
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
		notice   string
	}{
		{name: "item: newest Claude session", claude: []string{"fix~1", "review~1"}, want: "fix~1"},
		{name: "item: skips sessions without Claude", claude: []string{"review~1"}, want: "review~1"},
		{name: "item: no Claude session", notice: "no Claude session for this item"},
		{name: "sessions screen: selected session", sessions: true, cursor: 1, claude: []string{"fix~1", "review~1"}, want: "review~1"},
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
			if v := ansi.Strip(m.View()); !strings.Contains(v, "AGENT VIEW · "+tt.want) {
				t.Errorf("view does not show the agent view of %s:\n%s", tt.want, v)
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
