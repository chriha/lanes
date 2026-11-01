package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/history"
	"github.com/chriha/lanes/internal/session"
)

// claudeHook runs the Claude Stop hook inside lanes session name with a payload carrying claudeID.
func claudeHook(t *testing.T, name, claudeID string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(app.StateDir(), "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LANES_SESSION", name)
	t.Setenv("LANES_STATE_DIR", app.StateDir())
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(`{"session_id":"` + claudeID + `","transcript_path":"/t/x.jsonl"}`); err != nil {
		t.Fatal(err)
	}
	w.Close()
	old := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = old; r.Close() }()
	session.RunHook("Stop")
}

// Every way lanes records a session's end carries the Claude session id the hooks saw, and forgets it after.
func TestEndEventCarriesClaudeSession(t *testing.T) {
	alive := session.Info{Name: "fix~1", Action: "fix", ItemKey: "1"}
	tests := []struct {
		name  string
		event string
		end   func(m *Model)
	}{
		{"finished", "finished", func(m *Model) {
			dead := alive
			dead.Dead = true
			m.onSessions(sessionsMsg{infos: []session.Info{dead}})
		}},
		{"failed", "failed", func(m *Model) {
			dead := alive
			dead.Dead, dead.Exit = true, 1
			m.onSessions(sessionsMsg{infos: []session.Info{dead}})
		}},
		{"killed", "killed", func(m *Model) {
			press(m, "x")
			settle(m, press(m, "y"))
		}},
		{"interrupted", "interrupted", func(m *Model) {
			press(m, "X")
			settle(m, press(m, "y"))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := busyModel(t)
			paneTmux(t, 0)
			mgr, err := session.NewManager()
			if err != nil {
				t.Fatal(err)
			}
			m.sess = mgr
			m.sessions = []session.Info{alive}
			m.sessByItem["1"] = m.sessions
			claudeHook(t, alive.Name, "abc")

			tt.end(m)

			evs := history.For("1")
			if len(evs) == 0 {
				t.Fatal("no history event recorded")
			}
			last := evs[len(evs)-1]
			if last.Event != tt.event || last.ClaudeSession != "abc" {
				t.Errorf("last event = %+v, want %s with claude_session abc", last, tt.event)
			}
			if _, err := os.Stat(filepath.Join(app.StateDir(), "sessions", alive.Name+".claude.json")); !os.IsNotExist(err) {
				t.Errorf("Claude session file kept after the end event: %v", err)
			}
		})
	}
}

func TestEndEventWithoutClaude(t *testing.T) {
	m, _ := busyModel(t)
	alive := session.Info{Name: "fix~1", Action: "fix", ItemKey: "1"}
	m.sessions = []session.Info{alive}
	dead := alive
	dead.Dead = true
	m.onSessions(sessionsMsg{infos: []session.Info{dead}})
	evs := history.For("1")
	if len(evs) != 1 || evs[0].Event != "finished" || evs[0].ClaudeSession != "" {
		t.Errorf("history = %+v, want one finished event without claude_session", evs)
	}
}
