package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/session"
)

// fakeTmux puts a tmux on PATH that logs each call as one line and succeeds; it returns the log's path.
// TMUX is set so jumps take the popup path, which runs (the fake) tmux instead of suspending the TUI.
func fakeTmux(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	script := "#!/bin/sh\necho \"$*\" >> " + shq(log) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TMUX", "fake")
	return log
}

// settle runs cmd and feeds every message it yields back into the model, until no command is left.
func settle(m *Model, cmd tea.Cmd) {
	for _, msg := range runAll(cmd) {
		if _, ok := msg.(pollNow); ok {
			continue // would poll tmux; the tests set the sessions themselves
		}
		_, next := m.Update(msg)
		settle(m, next)
	}
}

func TestJumpAction(t *testing.T) {
	const attach = "attach -t '=review~1'"
	tests := []struct {
		name     string
		jump     bool
		sess     *session.Info // existing session of item 1
		marked   []string
		want     []string // substrings of the tmux calls, in order
		noAttach bool
		notice   string // expected notice, empty = not checked
	}{
		{name: "starts then attaches", jump: true,
			want: []string{"new-session -d -s review~1", attach}},
		{name: "running: attaches, no start", jump: true,
			sess: &session.Info{Name: "review~1", Action: "review", ItemKey: "1"},
			want: []string{attach}},
		{name: "dead: restarts then attaches", jump: true,
			sess: &session.Info{Name: "review~1", Action: "review", ItemKey: "1", Dead: true},
			want: []string{"kill-session -t =review~1", "new-session -d -s review~1", attach}},
		{name: "several marked: starts all, no attach", jump: true, marked: []string{"1", "2"},
			want: []string{"new-session -d -s review~1", "new-session -d -s review~2"}, noAttach: true},
		{name: "without jump: starts, no attach",
			want: []string{"new-session -d -s review~1"}, noAttach: true},
		{name: "without jump, running: notice, no attach",
			sess: &session.Info{Name: "review~1", Action: "review", ItemKey: "1"},
			want: nil, noAttach: true, notice: "review already running"},
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
			if tt.sess != nil {
				m.sessions = []session.Info{*tt.sess}
				m.sessByItem["1"] = m.sessions
			}
			for _, id := range tt.marked {
				m.marks[id] = true
			}
			cmd := m.runAction("review", config.Action{Run: "echo review", Jump: tt.jump})
			if len(tt.marked) > 1 {
				if m.mode != modeConfirm {
					t.Fatal("several items did not ask first")
				}
				cmd = press(m, "y")
			}
			settle(m, cmd)

			raw, _ := os.ReadFile(log)
			calls := strings.Split(strings.TrimSpace(string(raw)), "\n")
			if len(raw) == 0 {
				calls = nil
			}
			if len(calls) != len(tt.want) {
				t.Fatalf("tmux calls:\n%s\nwant %d matching %q", raw, len(tt.want), tt.want)
			}
			for i, w := range tt.want {
				if !strings.Contains(calls[i], w) {
					t.Errorf("call %d = %q, want it to contain %q", i, calls[i], w)
				}
			}
			if tt.noAttach && strings.Contains(string(raw), " attach -t ") {
				t.Errorf("attached, want no attach:\n%s", raw)
			}
			if strings.Contains(m.notice, "already running") && tt.notice == "" {
				t.Errorf("notice = %q, want no already-running notice", m.notice)
			}
			if tt.notice != "" && m.notice != tt.notice {
				t.Errorf("notice = %q, want %q", m.notice, tt.notice)
			}
		})
	}
}

func TestRunningItemsKeepTheirMark(t *testing.T) {
	m, _ := busyModel(t)
	fakeTmux(t)
	mgr, err := session.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	m.sess = mgr
	m.sessions = []session.Info{{Name: "review~1", Action: "review", ItemKey: "1"}}
	m.sessByItem["1"] = m.sessions
	m.marks["1"], m.marks["2"] = true, true // 1 already runs, 2 starts
	m.runAction("review", config.Action{Run: "echo review"})
	if m.mode != modeConfirm {
		t.Fatal("several items did not ask first")
	}
	settle(m, press(m, "y"))
	if len(m.marks) != 1 || !m.marks["1"] {
		t.Errorf("marks = %v, want only the running item 1", m.marks)
	}
}
