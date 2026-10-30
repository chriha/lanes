package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/session"
)

func TestShellKey(t *testing.T) {
	const attach = "attach -t '=shell~1'"
	tests := []struct {
		name   string
		sess   *session.Info // existing shell session of item 1 (the cursor item)
		marked []string
		keys   []string
		want   []string // substrings of the tmux calls, in order
	}{
		{name: "no shell: starts then attaches", keys: []string{"t"},
			want: []string{"new-session -d -s shell~1", attach}},
		{name: "live shell: attaches, no start", keys: []string{"t"},
			sess: &session.Info{Name: "shell~1", Action: "shell", ItemKey: "1"},
			want: []string{attach}},
		{name: "finished shell: restarts then attaches", keys: []string{"t"},
			sess: &session.Info{Name: "shell~1", Action: "shell", ItemKey: "1", Dead: true},
			want: []string{"kill-session -t =shell~1", "new-session -d -s shell~1", attach}},
		{name: "marks ignored: cursor item only", keys: []string{"t"}, marked: []string{"2", "3"},
			want: []string{"new-session -d -s shell~1", attach}},
		{name: "live shell, marks kept", keys: []string{"t"}, marked: []string{"2", "3"},
			sess: &session.Info{Name: "shell~1", Action: "shell", ItemKey: "1"},
			want: []string{attach}},
		{name: "x closes the shell after confirming", keys: []string{"x", "y"},
			sess: &session.Info{Name: "shell~1", Action: "shell", ItemKey: "1", Dead: true},
			want: []string{"kill-session -t =shell~1"}},
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
			settle(m, press(m, tt.keys...))

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
			for _, c := range calls {
				if strings.HasPrefix(c, "new-session") {
					if !strings.Contains(c, `exec "${SHELL:-/bin/sh}"`) {
						t.Errorf("shell session runs %q, want an interactive $SHELL", c)
					}
					if strings.Contains(c, " -c ") {
						t.Errorf("shell session sets a cwd, want lanes' cwd: %q", c)
					}
				}
			}
			for _, id := range tt.marked {
				if !m.marks[id] {
					t.Errorf("mark on %s dropped, want marks left alone", id)
				}
			}
		})
	}
}

func TestShellKeyShadowsKeymap(t *testing.T) {
	m, _ := busyModel(t)
	m.cfg.Keymap = map[string]string{"t": "review"}
	m.cfg.Action = map[string]config.Action{"review": {Run: "echo review"}}
	if !strings.Contains(strings.Join(m.helpLines(), "\n"), "shadowed by a built-in key") {
		t.Error("a [keymap] binding on t is not shown as shadowed")
	}
}
