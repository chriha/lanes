package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/session"
)

// The restart decision is taken from the session as it is when the confirm prompt is answered, not as it was
// when the prompt was shown: a session that came alive meanwhile is not killed, one that is gone is not killed
// either and is started.
func TestRestartDecidedAtConfirm(t *testing.T) {
	tests := []struct {
		name      string
		now       []string // panes tmux lists when the prompt is confirmed
		wantCalls []string // substrings of the tmux calls, in order
	}{
		{name: "still dead: killed and restarted", now: []string{deadPane("review~1")},
			wantCalls: []string{"kill-session -t =review~1", "new-session -d -s review~1"}},
		{name: "came alive: left alone", now: []string{alivePane("review~1")}},
		{name: "gone: started without a kill", wantCalls: []string{"new-session -d -s review~1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := busyModel(t)
			log := fakeTmux(t)
			mgr, err := session.NewManager()
			if err != nil {
				t.Fatal(err)
			}
			m.sess = mgr
			dead := session.Info{Name: "review~1", Action: "review", ItemKey: "1", Dead: true}
			m.sessions = []session.Info{dead}
			m.sessByItem["1"] = m.sessions
			setPanes(t, log, deadPane("review~1"))
			m.marks["1"], m.marks["2"] = true, true

			m.runAction("review", config.Action{Run: "echo review"})
			if m.mode != modeConfirm {
				t.Fatal("several items did not ask first")
			}
			setPanes(t, log, tt.now...) // the state changes while the prompt is open
			settle(m, press(m, "y"))

			raw, _ := os.ReadFile(log)
			var calls []string
			for _, c := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
				if strings.Contains(c, " kill-session ") || strings.Contains(c, " new-session -d -s review~1") {
					calls = append(calls, c)
				}
			}
			if len(calls) != len(tt.wantCalls) {
				t.Fatalf("tmux calls for review~1:\n%s\nwant %q", strings.Join(calls, "\n"), tt.wantCalls)
			}
			for i, w := range tt.wantCalls {
				if !strings.Contains(calls[i], w) {
					t.Errorf("call %d = %q, want it to contain %q", i, calls[i], w)
				}
			}
		})
	}
}
