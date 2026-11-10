package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

// Every way lanes records a session's end carries the Claude session id the hooks saw. A killed session is gone and
// its ids with it; a finished or failed one stays listed and keeps them for the agent view.
func TestEndEventCarriesClaudeSession(t *testing.T) {
	alive := session.Info{Name: "fix~1", Action: "fix", ItemKey: "1"}
	tests := []struct {
		name  string
		event string
		gone  bool // the session is gone, so are its Claude ids
		end   func(m *Model)
	}{
		{"finished", "finished", false, func(m *Model) {
			dead := alive
			dead.Dead = true
			m.onSessions(sessionsMsg{infos: []session.Info{dead}})
		}},
		{"failed", "failed", false, func(m *Model) {
			dead := alive
			dead.Dead, dead.Exit = true, 1
			m.onSessions(sessionsMsg{infos: []session.Info{dead}})
		}},
		{"killed", "killed", true, func(m *Model) {
			press(m, "x")
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
			_, err = os.Stat(filepath.Join(app.StateDir(), "sessions", alive.Name+".claude.json"))
			if tt.gone != os.IsNotExist(err) {
				t.Errorf("Claude session file after the end event: err %v, want gone %v", err, tt.gone)
			}
		})
	}
}

// An interrupt only signals the session, which may keep running: it records the Claude session id but keeps it,
// so a later end without a hook event in between still carries it.
func TestInterruptKeepsClaudeSession(t *testing.T) {
	m, _ := busyModel(t)
	paneTmux(t, 0)
	mgr, err := session.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	m.sess = mgr
	alive := session.Info{Name: "fix~1", Action: "fix", ItemKey: "1"}
	m.sessions = []session.Info{alive}
	m.sessByItem["1"] = m.sessions
	claudeHook(t, alive.Name, "abc")
	sidecar := filepath.Join(app.StateDir(), "sessions", alive.Name+".claude.json")

	press(m, "X")
	settle(m, press(m, "y"))
	if _, err := os.Stat(sidecar); err != nil {
		t.Errorf("Claude session file gone after an interrupt: %v", err)
	}

	press(m, "x")
	settle(m, press(m, "y"))
	evs := history.For("1")
	if len(evs) != 2 {
		t.Fatalf("history = %+v, want interrupted then killed", evs)
	}
	for i, want := range []string{"interrupted", "killed"} {
		if evs[i].Event != want || evs[i].ClaudeSession != "abc" {
			t.Errorf("event %d = %+v, want %s with claude_session abc", i, evs[i], want)
		}
	}
	if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
		t.Errorf("Claude session file kept after the session ended: %v", err)
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

// fixClock makes clock return t for the test's duration.
func fixClock(t *testing.T, at time.Time) {
	t.Helper()
	old := clock
	clock = func() time.Time { return at }
	t.Cleanup(func() { clock = old })
}

// Every session end event carries how long the session ran: from its tmux creation to the observed end.
func TestEndEventCarriesDuration(t *testing.T) {
	end := time.Date(2026, 11, 1, 15, 4, 0, 0, time.UTC)
	alive := session.Info{Name: "fix~1", Action: "fix", ItemKey: "1", Created: end.Add(-90 * time.Second)}
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
			fixClock(t, end)

			tt.end(m)

			evs := history.For("1")
			if len(evs) == 0 {
				t.Fatal("no history event recorded")
			}
			if last := evs[len(evs)-1]; last.Event != tt.event || last.Dur != 90 {
				t.Errorf("last event = %+v, want %s with dur 90", last, tt.event)
			}
		})
	}
}

// A session whose creation time is unknown records no duration rather than one since year 1.
func TestEndEventWithoutCreatedHasNoDuration(t *testing.T) {
	m, _ := busyModel(t)
	alive := session.Info{Name: "fix~1", Action: "fix", ItemKey: "1"}
	m.sessions = []session.Info{alive}
	dead := alive
	dead.Dead = true
	m.onSessions(sessionsMsg{infos: []session.Info{dead}})
	if evs := history.For("1"); len(evs) != 1 || evs[0].Dur != 0 {
		t.Errorf("history = %+v, want one event without dur", evs)
	}
}

// A run's end event carries the time measured around its command.
func TestRunEndEventCarriesDuration(t *testing.T) {
	tests := []struct {
		name  string
		cmd   string
		event string
	}{
		{"finished", "true", "finished"},
		{"failed", "exit 3", "failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			t.Setenv("SHELL", "/bin/sh")
			t0 := time.Date(2026, 11, 1, 15, 4, 0, 0, time.UTC)
			calls := 0
			old := clock
			clock = func() time.Time { calls++; return t0.Add(time.Duration(calls-1) * 42 * time.Second) } // one job: no concurrent calls
			t.Cleanup(func() { clock = old })

			m := &Model{}
			m.executeRun("act", []job{{label: "a", itemKey: "k", cwd: t.TempDir(), cmd: tt.cmd}}, false, 0, "")()

			evs := history.For("k")
			if len(evs) != 2 {
				t.Fatalf("history = %+v, want started and end", evs)
			}
			if evs[0].Dur != 0 || evs[1].Event != tt.event || evs[1].Dur != 42 {
				t.Errorf("history = %+v, want started without dur, then %s with dur 42", evs, tt.event)
			}
		})
	}
}
