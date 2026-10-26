package session

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chriha/lanes/internal/app"
)

// fakeRun returns a Manager whose tmux calls are answered by fn and recorded in *calls.
func fakeRun(calls *[][]string, fn func(args []string) ([]byte, error)) *Manager {
	return &Manager{conf: "/conf", run: func(args ...string) ([]byte, error) {
		if calls != nil {
			*calls = append(*calls, args)
		}
		return fn(args)
	}}
}

func stateEnv(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(app.StateDir(), "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LANES_STATE_DIR", app.StateDir())
}

func TestKeyName(t *testing.T) {
	tests := []struct{ id, key string }{
		{"123", "123"},
		{"group/proj#12", "group_proj_12"},
		{"a b.c:d", "a_b_c_d"},
		{"ok-id_1", "ok-id_1"},
	}
	for _, tt := range tests {
		if got := Key(tt.id); got != tt.key {
			t.Errorf("Key(%q) = %q, want %q", tt.id, got, tt.key)
		}
		name := Name("review", tt.id)
		action, key, ok := strings.Cut(name, "~")
		if !ok || action != "review" || key != tt.key {
			t.Errorf("Name round trip: %q -> %q %q %v", name, action, key, ok)
		}
	}
}

func TestShq(t *testing.T) {
	tests := []struct{ in, want string }{
		{"abc", `'abc'`},
		{"", `''`},
		{"a b", `'a b'`},
		{"it's", `'it'\''s'`},
		{"$(rm -rf /)", `'$(rm -rf /)'`},
	}
	for _, tt := range tests {
		if got := shq(tt.in); got != tt.want {
			t.Errorf("shq(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBadge(t *testing.T) {
	tests := []struct {
		name       string
		in         Info
		text, kind string
	}{
		{"dead ok", Info{Action: "fix", Dead: true}, "✓ fix", "ok"},
		{"dead failed", Info{Action: "fix", Dead: true, Exit: 2}, "✗ fix (2)", "bad"},
		{"working", Info{Action: "fix", Claude: "working"}, "◐ fix: working", "info"},
		{"asking", Info{Action: "fix", Claude: "asking"}, "? fix: asking", "warn"},
		{"waiting", Info{Action: "fix", Claude: "waiting"}, "● fix: waiting", "warn"},
		{"running", Info{Action: "fix"}, "▶ fix", "info"},
		{"dead wins over claude", Info{Action: "fix", Dead: true, Claude: "working"}, "✓ fix", "ok"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := tt.in.Badge()
			if b.Text != tt.text || b.Kind != tt.kind {
				t.Errorf("got %+v, want %q/%q", b, tt.text, tt.kind)
			}
		})
	}
}

func TestJumpCmd(t *testing.T) {
	m := &Manager{conf: "/my conf"}
	tests := []struct {
		name     string
		tmuxEnv  string
		popup    bool
		wantArgs []string // prefix of Args
		inAttach string   // substring of the last arg, popup only
	}{
		{"outside tmux", "", false, []string{"tmux", "-L", app.Socket(), "-f", "/my conf", "attach", "-t", "=fix~1"}, ""},
		{"inside tmux", "/tmp/tmux-1/default,1,0", true, []string{"tmux", "display-popup", "-E"}, "env -u TMUX tmux -L " + app.Socket() + " -f '/my conf' attach -t '=fix~1'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TMUX", tt.tmuxEnv)
			cmd, popup := m.JumpCmd("fix~1")
			if popup != tt.popup {
				t.Errorf("popup = %v, want %v", popup, tt.popup)
			}
			if !slices.Equal(cmd.Args[:len(tt.wantArgs)], tt.wantArgs) {
				t.Errorf("args = %q, want prefix %q", cmd.Args, tt.wantArgs)
			}
			if last := cmd.Args[len(cmd.Args)-1]; tt.inAttach != "" && last != tt.inAttach {
				t.Errorf("attach = %q, want %q", last, tt.inAttach)
			}
		})
	}
}

func TestList(t *testing.T) {
	stateEnv(t)
	tests := []struct {
		name string
		out  string
		err  error
		want []Info
	}{
		{"no server", "", errors.New("exit status 1"), nil},
		{"empty output", "", nil, nil},
		{"malformed lines skipped", "garbage\nfix~1\t0\t0\n\nnotab\t0\t0\t5\nfix~2\t0\t0\t5\n", nil,
			[]Info{{Name: "fix~2", Action: "fix", ItemKey: "2", Created: time.Unix(5, 0)}}},
		{"dead with exit status, newest first",
			"fix~1\t1\t3\t100\nreview~2\t0\t0\t200\n", nil,
			[]Info{{Name: "review~2", Action: "review", ItemKey: "2", Created: time.Unix(200, 0)},
				{Name: "fix~1", Action: "fix", ItemKey: "1", Dead: true, Exit: 3, Created: time.Unix(100, 0)}}},
		{"item key keeps later tildes", "fix~a~b\t0\t0\t1\n", nil,
			[]Info{{Name: "fix~a~b", Action: "fix", ItemKey: "a~b", Created: time.Unix(1, 0)}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls [][]string
			m := fakeRun(&calls, func([]string) ([]byte, error) { return []byte(tt.out), tt.err })
			got, err := m.List()
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			for i := range got {
				w := tt.want[i]
				if got[i] != w {
					t.Errorf("[%d] got %+v, want %+v", i, got[i], w)
				}
			}
			if len(calls) != 1 || calls[0][0] != "list-panes" {
				t.Errorf("calls = %q, want one list-panes", calls)
			}
		})
	}
}

func TestStartKillArgs(t *testing.T) {
	stateEnv(t)
	t.Setenv("SHELL", "/bin/zsh")
	var calls [][]string
	m := fakeRun(&calls, func([]string) ([]byte, error) { return nil, nil })
	if err := m.Start("fix~1", "/work", "echo hi"); err != nil {
		t.Fatal(err)
	}
	if err := m.Kill("fix~1"); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %q", calls)
	}
	start := calls[0]
	if !slices.Contains(start, "new-session") || !slices.Contains(start, "/work") || !slices.Contains(start, "/bin/zsh") {
		t.Errorf("start args = %q", start)
	}
	if script := start[len(start)-1]; !strings.Contains(script, "LANES_SESSION='fix~1'") || !strings.HasSuffix(script, "echo hi") {
		t.Errorf("script = %q", script)
	}
	if want := []string{"kill-session", "-t", "=fix~1"}; !slices.Equal(calls[1], want) {
		t.Errorf("kill args = %q, want %q", calls[1], want)
	}
}

func TestStartError(t *testing.T) {
	stateEnv(t)
	m := fakeRun(nil, func([]string) ([]byte, error) { return []byte("duplicate session\n"), errors.New("exit status 1") })
	err := m.Start("fix~1", "", "x")
	if err == nil || err.Error() != "tmux: duplicate session" {
		t.Errorf("err = %v", err)
	}
}

// feedStdin replaces os.Stdin with a pipe carrying s for the duration of the test.
func feedStdin(t *testing.T, s string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(s); err != nil {
		t.Fatal(err)
	}
	w.Close()
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old; r.Close() })
}

func TestHookRoundTrip(t *testing.T) {
	tests := []struct {
		name, event, stdin, wantState, wantBadge string
	}{
		{"prompt", "UserPromptSubmit", "{}", "working", "◐ fix: working"},
		{"tool", "PreToolUse", "{}", "working", "◐ fix: working"},
		{"stop", "Stop", "", "waiting", "● fix: waiting"},
		{"notification asks", "Notification", `{"notification_type":"permission_prompt"}`, "asking", "? fix: asking"},
		{"idle notification waits", "Notification", `{"notification_type":"idle_prompt"}`, "waiting", "● fix: waiting"},
		{"unknown event ignored", "SessionStart", "{}", "", "▶ fix"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stateEnv(t)
			t.Setenv("LANES_SESSION", "fix~1")
			feedStdin(t, tt.stdin)
			RunHook(tt.event)

			m := fakeRun(nil, func([]string) ([]byte, error) {
				return []byte("fix~1\t0\t0\t10\n"), nil
			})
			got, err := m.List()
			if err != nil || len(got) != 1 {
				t.Fatalf("List = %+v, %v", got, err)
			}
			if got[0].Claude != tt.wantState {
				t.Errorf("Claude = %q, want %q", got[0].Claude, tt.wantState)
			}
			if b := got[0].Badge(); b.Text != tt.wantBadge {
				t.Errorf("badge = %q, want %q", b.Text, tt.wantBadge)
			}
		})
	}
}

func TestHookNoSessionWritesNothing(t *testing.T) {
	stateEnv(t)
	t.Setenv("LANES_SESSION", "")
	feedStdin(t, "{}")
	RunHook("Stop")
	files, _ := filepath.Glob(filepath.Join(app.StateDir(), "sessions", "*"))
	if len(files) != 0 {
		t.Errorf("unexpected state files: %q", files)
	}
}

func TestListIgnoresStateOfDeadSession(t *testing.T) {
	stateEnv(t)
	t.Setenv("LANES_SESSION", "fix~1")
	feedStdin(t, "{}")
	RunHook("Stop")
	m := fakeRun(nil, func([]string) ([]byte, error) { return []byte("fix~1\t1\t0\t10\n"), nil })
	got, _ := m.List()
	if len(got) != 1 || got[0].Claude != "" {
		t.Errorf("got %+v", got)
	}
}

func TestKillRemovesState(t *testing.T) {
	stateEnv(t)
	t.Setenv("LANES_SESSION", "fix~1")
	feedStdin(t, "{}")
	RunHook("Stop")
	if readState("fix~1") != "waiting" {
		t.Fatal("state not written")
	}
	m := fakeRun(nil, func([]string) ([]byte, error) { return nil, nil })
	if err := m.Kill("fix~1"); err != nil {
		t.Fatal(err)
	}
	if got := readState("fix~1"); got != "" {
		t.Errorf("state after kill = %q", got)
	}
}

func TestCount(t *testing.T) {
	tests := []struct {
		name           string
		in             []Info
		run, ask, wait int
	}{
		{"none", nil, 0, 0, 0},
		{"dead not counted", []Info{{Dead: true}, {Dead: true, Exit: 1, Claude: "asking"}}, 0, 0, 0},
		{"no claude status is running", []Info{{}}, 1, 0, 0},
		{"working is running", []Info{{Claude: "working"}}, 1, 0, 0},
		{"asking", []Info{{Claude: "asking"}, {Claude: "asking"}}, 0, 2, 0},
		{"waiting", []Info{{Claude: "waiting"}}, 0, 0, 1},
		{"mixed", []Info{{Claude: "working"}, {Claude: "asking"}, {Claude: "waiting"}, {Claude: "waiting"}, {Dead: true}}, 1, 1, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, ask, wait := Count(tt.in)
			if run != tt.run || ask != tt.ask || wait != tt.wait {
				t.Errorf("Count = %d %d %d, want %d %d %d", run, ask, wait, tt.run, tt.ask, tt.wait)
			}
		})
	}
}

// TestStatusLine goes through List with the fake runner, so the line reflects what tmux and the hook state report.
func TestStatusLine(t *testing.T) {
	tests := []struct {
		name   string
		out    string
		err    error
		states map[string]string
		want   string
	}{
		{"no server", "", errors.New("no server running"), nil, ""},
		{"only running", "fix~1\t0\t0\t1\n", nil, map[string]string{"fix~1": "working"}, ""},
		{"dead asking ignored", "fix~1\t1\t0\t1\n", nil, map[string]string{"fix~1": "asking"}, ""},
		{"asking only", "fix~1\t0\t0\t1\n", nil, map[string]string{"fix~1": "asking"}, "? 1"},
		{"waiting only", "fix~1\t0\t0\t1\n", nil, map[string]string{"fix~1": "waiting"}, "● 1"},
		{"both", "fix~1\t0\t0\t1\nfix~2\t0\t0\t2\nfix~3\t0\t0\t3\nfix~4\t0\t0\t4\n", nil,
			map[string]string{"fix~1": "asking", "fix~2": "waiting", "fix~3": "waiting", "fix~4": "working"}, "? 1 ● 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stateEnv(t)
			for name, state := range tt.states {
				if err := os.WriteFile(statePath(name), []byte(`{"state":"`+state+`"}`), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			m := fakeRun(nil, func([]string) ([]byte, error) { return []byte(tt.out), tt.err })
			infos, err := m.List()
			if err != nil {
				t.Fatal(err)
			}
			if got := StatusLine(infos); got != tt.want {
				t.Errorf("StatusLine = %q, want %q", got, tt.want)
			}
		})
	}
}
