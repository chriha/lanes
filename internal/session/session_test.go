package session

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
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
		{"malformed lines skipped", "garbage\nfix~1\t0\t0\n\nnotab\t0\t0\t5\nfix~2\t0\t0\t5\tclaude\n", nil,
			[]Info{{Name: "fix~2", Action: "fix", ItemKey: "2", Created: time.Unix(5, 0)}}},
		{"dead with exit status, newest first",
			"fix~1\t1\t3\t100\tzsh\nreview~2\t0\t0\t200\tclaude\n", nil,
			[]Info{{Name: "review~2", Action: "review", ItemKey: "2", Created: time.Unix(200, 0)},
				{Name: "fix~1", Action: "fix", ItemKey: "1", Dead: true, Exit: 3, Created: time.Unix(100, 0)}}},
		{"killed by a signal fails with 128+n",
			"fix~1\t1\tsigterm\t1\tzsh\nfix~2\t1\tsig9\t2\tzsh\nfix~3\t1\tsigusr1\t3\tzsh\n", nil,
			[]Info{{Name: "fix~3", Action: "fix", ItemKey: "3", Dead: true, Exit: 128, Created: time.Unix(3, 0)},
				{Name: "fix~2", Action: "fix", ItemKey: "2", Dead: true, Exit: 137, Created: time.Unix(2, 0)},
				{Name: "fix~1", Action: "fix", ItemKey: "1", Dead: true, Exit: 143, Created: time.Unix(1, 0)}}},
		{"item key keeps later tildes", "fix~a~b\t0\t0\t1\tclaude\n", nil,
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
		{"session end clears", "SessionEnd", "{}", "", "▶ fix"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stateEnv(t)
			t.Setenv("LANES_SESSION", "fix~1")
			feedStdin(t, tt.stdin)
			RunHook(tt.event)

			m := fakeRun(nil, func([]string) ([]byte, error) {
				return []byte("fix~1\t0\t0\t10\tclaude\n"), nil
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
	m := fakeRun(nil, func([]string) ([]byte, error) { return []byte("fix~1\t1\t0\t10\tzsh\n"), nil })
	got, _ := m.List()
	if len(got) != 1 || got[0].Claude != "" {
		t.Errorf("got %+v", got)
	}
}

func TestListIgnoresStateWhileShell(t *testing.T) {
	tests := []struct {
		name, shellEnv, command, wantClaude string
	}{
		{"zsh", "/bin/zsh", "zsh", ""},
		{"bash", "", "bash", ""},
		{"sh", "", "sh", ""},
		{"fish", "", "fish", ""},
		{"login shell from SHELL", "/opt/homebrew/bin/nu", "nu", ""},
		{"claude", "/bin/zsh", "claude", "asking"},
		{"node", "/bin/zsh", "node", "asking"},
		{"other program", "/bin/zsh", "python3", "asking"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stateEnv(t)
			t.Setenv("SHELL", tt.shellEnv)
			t.Setenv("LANES_SESSION", "fix~1")
			feedStdin(t, `{"notification_type":"permission_prompt"}`)
			RunHook("Notification")
			m := fakeRun(nil, func([]string) ([]byte, error) { return []byte("fix~1\t0\t0\t10\t" + tt.command + "\n"), nil })
			got, err := m.List()
			if err != nil || len(got) != 1 {
				t.Fatalf("List = %+v, %v", got, err)
			}
			if got[0].Claude != tt.wantClaude {
				t.Errorf("Claude = %q, want %q", got[0].Claude, tt.wantClaude)
			}
			run, ask, wait := Count(got)
			wantRun, wantAsk := 1, 0
			if tt.wantClaude == "asking" {
				wantRun, wantAsk = 0, 1
			}
			if run != wantRun || ask != wantAsk || wait != 0 {
				t.Errorf("Count = %d,%d,%d", run, ask, wait)
			}
		})
	}
}

func TestSessionEndRemovesState(t *testing.T) {
	tests := []struct {
		name, stdin string
		wantGone    bool
	}{
		{"no reason", "{}", true},
		{"exit", `{"reason":"prompt_input_exit"}`, true},
		{"logout", `{"reason":"logout"}`, true},
		{"clear keeps state", `{"reason":"clear"}`, false},
		{"resume keeps state", `{"reason":"resume"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stateEnv(t)
			t.Setenv("LANES_SESSION", "fix~1")
			feedStdin(t, "{}")
			RunHook("Stop")
			if _, err := os.Stat(statePath("fix~1")); err != nil {
				t.Fatalf("state not written: %v", err)
			}
			feedStdin(t, tt.stdin)
			RunHook("SessionEnd")
			_, err := os.Stat(statePath("fix~1"))
			if gone := os.IsNotExist(err); gone != tt.wantGone {
				t.Errorf("state removed = %v, want %v (err %v)", gone, tt.wantGone, err)
			}
		})
	}
}

func TestSettingsFileRegistersSessionEnd(t *testing.T) {
	stateEnv(t)
	p, err := SettingsFile()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Hooks map[string][]struct {
			Hooks []struct{ Command string }
		}
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	h := s.Hooks["SessionEnd"]
	if len(h) != 1 || len(h[0].Hooks) != 1 || !strings.HasSuffix(h[0].Hooks[0].Command, " hook SessionEnd") {
		t.Errorf("SessionEnd hook = %+v", h)
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
		{"only running", "fix~1\t0\t0\t1\tclaude\n", nil, map[string]string{"fix~1": "working"}, ""},
		{"dead asking ignored", "fix~1\t1\t0\t1\tclaude\n", nil, map[string]string{"fix~1": "asking"}, ""},
		{"asking only", "fix~1\t0\t0\t1\tclaude\n", nil, map[string]string{"fix~1": "asking"}, "? 1"},
		{"waiting only", "fix~1\t0\t0\t1\tclaude\n", nil, map[string]string{"fix~1": "waiting"}, "● 1"},
		{"both", "fix~1\t0\t0\t1\tclaude\nfix~2\t0\t0\t2\tclaude\nfix~3\t0\t0\t3\tclaude\nfix~4\t0\t0\t4\tclaude\n", nil,
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

func TestPrune(t *testing.T) {
	// fix~1 running, fix~2 dead ok, fix~3 dead failed, review~4 dead failed.
	const panes = "fix~1\t0\t0\t1\tclaude\nfix~2\t1\t0\t2\tclaude\nfix~3\t1\t2\t3\tclaude\nreview~4\t1\t1\t4\tclaude\n"
	tests := []struct {
		name       string
		out        string
		listErr    error
		failedOnly bool
		failKill   string // session whose kill-session fails
		wantKills  []string
		wantN      int
		wantErr    bool
	}{
		{"all dead, no running", panes, nil, false, "", []string{"review~4", "fix~3", "fix~2"}, 3, false},
		{"failed only", panes, nil, true, "", []string{"review~4", "fix~3"}, 2, false},
		{"failing kill does not stop the others", panes, nil, false, "fix~3", []string{"review~4", "fix~3", "fix~2"}, 2, true},
		{"only running", "fix~1\t0\t0\t1\tclaude\n", nil, false, "", nil, 0, false},
		{"no server", "", errors.New("no server running"), false, "", nil, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stateEnv(t)
			var kills []string
			m := fakeRun(nil, func(args []string) ([]byte, error) {
				switch args[0] {
				case "list-panes":
					return []byte(tt.out), tt.listErr
				case "kill-session":
					name := strings.TrimPrefix(args[2], "=")
					kills = append(kills, name)
					if name == tt.failKill {
						return []byte("can't kill"), errors.New("exit status 1")
					}
					return nil, nil
				}
				t.Fatalf("unexpected tmux call %q", args)
				return nil, nil
			})
			n, err := m.Prune(tt.failedOnly)
			if n != tt.wantN || (err != nil) != tt.wantErr {
				t.Errorf("Prune = %d, %v; want %d, error %v", n, err, tt.wantN, tt.wantErr)
			}
			if !slices.Equal(kills, tt.wantKills) {
				t.Errorf("killed %q, want %q", kills, tt.wantKills)
			}
		})
	}
}

func TestUsage(t *testing.T) {
	tests := []struct {
		name, dir, file, want string
		wantAt                int64
	}{
		{"no dir configured", "", `{"text":"5h 23%","at":100}`, "", 0},
		{"reads text and time", "DIR", `{"text":"5h 23% · wk 30%","at":100}`, "5h 23% · wk 30%", 100},
		{"tilde is the home dir", "~/.claude-personal", `{"text":"5h 1%","at":5}`, "5h 1%", 5},
		{"missing file", "DIR", "", "", 0},
		{"empty text", "DIR", `{"text":null,"at":100}`, "", 0},
		{"broken json", "DIR", `{`, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			dir, real := tt.dir, filepath.Join(home, ".claude")
			if dir == "DIR" {
				dir = real
			} else if rest, ok := strings.CutPrefix(dir, "~"); ok {
				real = home + rest
			}
			if tt.file != "" {
				if err := os.MkdirAll(real, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(real, UsageFile), []byte(tt.file), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			text, at := Usage(dir)
			if text != tt.want {
				t.Errorf("text = %q, want %q", text, tt.want)
			}
			if tt.want != "" && at.Unix() != tt.wantAt {
				t.Errorf("at = %d, want %d", at.Unix(), tt.wantAt)
			}
		})
	}
}

func TestInterrupt(t *testing.T) {
	// pane shell 100; its children 200 (own group) and 300+301 (one group 300); 400 is unrelated
	const ps = "  1     0     1\n100     1   100\n200   100   200\n300   100   300\n301   100   300\n400     1   400\n"
	type sent struct {
		pgid int
		sig  syscall.Signal
	}
	tests := []struct {
		name     string
		pane     string // display-message output: "<dead> <pid>"
		tmuxErr  error
		ps       string
		psErr    error
		sigErr   error
		force    bool
		wantSent []sent
		wantErr  bool
		wantSig  bool // reported as signalled
	}{
		{"terms each child's group once", "0 100", nil, ps, nil, nil, false, []sent{{200, syscall.SIGTERM}, {300, syscall.SIGTERM}}, false, true},
		{"force kills", "0 100", nil, ps, nil, nil, true, []sent{{200, syscall.SIGKILL}, {300, syscall.SIGKILL}}, false, true},
		{"no children: the pane runs the command itself", "0 400", nil, ps, nil, nil, false, []sent{{400, syscall.SIGTERM}}, false, true},
		{"dead pane is left alone", "1 100", nil, ps, nil, nil, false, nil, false, false},
		{"tmux error surfaced", "", errors.New("no such session"), ps, nil, nil, false, nil, true, false},
		{"bad pane pid", "0 abc", nil, ps, nil, nil, false, nil, true, false},
		{"ps error surfaced", "0 100", nil, "", errors.New("ps failed"), nil, false, nil, true, false},
		{"signal error surfaced", "0 100", nil, ps, nil, syscall.EPERM, false, []sent{{200, syscall.SIGTERM}, {300, syscall.SIGTERM}}, true, false},
		{"already gone is no error", "0 100", nil, ps, nil, syscall.ESRCH, false, []sent{{200, syscall.SIGTERM}, {300, syscall.SIGTERM}}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls [][]string
			m := fakeRun(&calls, func([]string) ([]byte, error) { return []byte(tt.pane + "\n"), tt.tmuxErr })
			m.ps = func() ([]byte, error) { return []byte(tt.ps), tt.psErr }
			var got []sent
			m.signal = func(pgid int, sig syscall.Signal) error {
				got = append(got, sent{pgid, sig})
				return tt.sigErr
			}
			sig, err := m.Interrupt("fix~1", tt.force)
			if (err != nil) != tt.wantErr || sig != tt.wantSig {
				t.Errorf("signalled %v, err %v; want signalled %v, error %v", sig, err, tt.wantSig, tt.wantErr)
			}
			if !slices.Equal(got, tt.wantSent) {
				t.Errorf("signals = %v, want %v", got, tt.wantSent)
			}
			if len(calls) != 1 || !slices.Equal(calls[0], []string{"display-message", "-p", "-t", "=fix~1:", "#{pane_dead} #{pane_pid}"}) {
				t.Errorf("tmux calls = %q", calls)
			}
		})
	}
}

func TestSettingsFileIsPrivate(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	p, err := SettingsFile()
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(filepath.Dir(p)); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("state dir = %v, %v; want 0700", fi, err)
	}
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := SettingsFile(); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("settings = %v, %v; want 0600", fi, err)
	}
}

func TestHookCommandQuotesExecutablePath(t *testing.T) {
	for _, exe := range []string{"/usr/bin/lanes", "/opt/my dir/it's $HOME `x`/lanes"} {
		out, err := exec.Command("sh", "-c", "printf '%s\\n' "+hookCommand(exe, "Stop")).Output()
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(out), exe+"\nhook\nStop\n"; got != want {
			t.Errorf("exe %q: words = %q, want %q", exe, got, want)
		}
	}
}
