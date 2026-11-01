package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/trust"
)

func TestUsageError(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"no args opens the TUI", nil, false},
		{"doctor", []string{"doctor"}, false},
		{"status", []string{"status"}, false},
		{"init", []string{"init"}, false},
		{"version", []string{"version"}, false},
		{"--version", []string{"--version"}, false},
		{"--untrust", []string{"--untrust"}, false},
		{"hook with event", []string{"hook", "Stop"}, false},
		{"doctor extra", []string{"doctor", "x"}, true},
		{"status extra", []string{"status", "x"}, true},
		{"init extra", []string{"init", "x"}, true},
		{"version extra", []string{"version", "x"}, true},
		{"--version extra", []string{"--version", "x"}, true},
		{"--untrust extra", []string{"--untrust", "x"}, true},
		{"hook without event", []string{"hook"}, true},
		{"hook two args", []string{"hook", "a", "b"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := usageError(tt.args); (got != "") != tt.wantErr {
				t.Errorf("usageError(%q) = %q, wantErr %v", tt.args, got, tt.wantErr)
			}
		})
	}
}

// fakeEnv returns an env for dir whose steps append their name to calls; ask answers yes.
func fakeEnv(dir string, tty, yes bool, calls *[]string) (env, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	rec := func(s string) { *calls = append(*calls, s) }
	return env{
		wd: dir, isTTY: tty, stdout: &stdout, stderr: &stderr,
		ask:     func(string) (bool, bool, error) { rec("ask"); return yes, false, nil },
		initApp: func(string) { rec("initApp") },
		hook:    func(string) { rec("hook") },
		status:  func() { rec("status") },
		doctor:  func() int { rec("doctor"); return 0 },
		init: func() int {
			rec("init")
			return 0
		},
		tui: func() int { rec("tui"); return 0 },
	}, &stdout, &stderr
}

func TestRunGate(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		trusted    bool // trust the project (with its current hash) before running
		tty, yes   bool
		wantCode   int
		wantCalls  string
		wantStderr string // substring; "" = stderr must be empty
		wantStored bool   // trust.json trusts the project afterwards
	}{
		{"tui untrusted, yes", nil, false, true, true, 0, "ask initApp tui", "", true},
		{"tui untrusted, no", nil, false, true, false, 1, "ask", "not trusted", false},
		{"tui untrusted, no tty", nil, false, false, true, 1, "", "%ROOT% is not trusted; run lanes in a terminal to trust it\n", false},
		{"tui trusted", nil, true, true, true, 0, "initApp tui", "", true},
		{"doctor untrusted, yes", []string{"doctor"}, false, true, true, 0, "ask initApp doctor", "", true},
		{"doctor untrusted, no", []string{"doctor"}, false, true, false, 1, "ask", "not trusted", false},
		{"init untrusted, no", []string{"init"}, false, true, false, 1, "ask", "not trusted", false},
		{"init untrusted, no tty", []string{"init"}, false, false, true, 1, "", "is not trusted", false},
		{"status untrusted", []string{"status"}, false, false, true, 0, "", "", false},
		{"status trusted", []string{"status"}, true, false, true, 0, "initApp status", "", true},
		{"hook untrusted", []string{"hook", "Stop"}, false, false, true, 0, "", "", false},
		{"hook trusted", []string{"hook", "Stop"}, true, false, true, 0, "hook", "", true},
		{"version untrusted", []string{"--version"}, false, false, true, 0, "", "", false},
		{"version word untrusted", []string{"version"}, false, false, true, 0, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			root := t.TempDir()
			dir := filepath.Join(root, "sub")
			if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, config.ProjectFile), []byte("# x\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			hash, _ := trust.HashRoot(root)
			if tt.trusted {
				if err := trust.Trust(root, hash); err != nil {
					t.Fatal(err)
				}
			}
			var calls []string
			e, stdout, stderr := fakeEnv(dir, tt.tty, tt.yes, &calls)
			if code := run(tt.args, e); code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if got := strings.Join(calls, " "); got != tt.wantCalls {
				t.Errorf("calls = %q, want %q", got, tt.wantCalls)
			}
			want := strings.ReplaceAll(tt.wantStderr, "%ROOT%", root)
			if want == "" && stderr.Len() > 0 || !strings.Contains(stderr.String(), want) {
				t.Errorf("stderr = %q, want %q", stderr.String(), want)
			}
			isVersion := len(tt.args) > 0 && strings.Contains(tt.args[0], "version")
			if isVersion != (stdout.Len() > 0) {
				t.Errorf("stdout = %q", stdout.String())
			}
			if ok, err := trust.IsTrusted(root, hash); err != nil || ok != tt.wantStored {
				t.Errorf("stored = %v (err %v), want %v", ok, err, tt.wantStored)
			}
		})
	}
}

// gateRoot makes a git root in a temp dir with a .lanes.toml in its sub dir and returns the root and the sub dir.
func gateRoot(t *testing.T) (root, dir string) {
	t.Helper()
	root = t.TempDir()
	dir = filepath.Join(root, "sub")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, config.ProjectFile), []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, dir
}

func TestRunStoresAlways(t *testing.T) {
	tests := []struct {
		name       string
		yes        bool
		always     bool // the dialog's checkbox
		wantCode   int
		wantCalls  string
		wantHash   bool // trust.json trusts the current hash afterwards
		wantAlways bool // trust.json trusts the root whatever the hash
	}{
		{"yes, unchecked stores the hash", true, false, 0, "ask initApp tui", true, false},
		{"yes, checked stores always", true, true, 0, "ask initApp tui", true, true},
		{"no, unchecked stores nothing", false, false, 1, "ask", false, false},
		{"no, checked stores nothing", false, true, 1, "ask", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			root, dir := gateRoot(t)
			var calls []string
			e, _, stderr := fakeEnv(dir, true, tt.yes, &calls)
			e.ask = func(string) (bool, bool, error) {
				calls = append(calls, "ask")
				return tt.yes, tt.always, nil
			}
			if code := run(nil, e); code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if got := strings.Join(calls, " "); got != tt.wantCalls {
				t.Errorf("calls = %q, want %q", got, tt.wantCalls)
			}
			if !tt.yes && !strings.Contains(stderr.String(), "not trusted") {
				t.Errorf("stderr = %q, want not trusted", stderr.String())
			}
			hash, _ := trust.HashRoot(root)
			if ok, err := trust.IsTrusted(root, hash); err != nil || ok != tt.wantHash {
				t.Errorf("hash trusted = %v (err %v), want %v", ok, err, tt.wantHash)
			}
			if ok, err := trust.IsTrusted(root, "other"); err != nil || ok != tt.wantAlways {
				t.Errorf("always trusted = %v (err %v), want %v", ok, err, tt.wantAlways)
			}
		})
	}
}

func TestRunAlwaysTrustedRootIgnoresChanges(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		tty       bool
		wantCalls string
	}{
		{"tui", nil, true, "initApp tui"},
		{"tui without tty", nil, false, "initApp tui"},
		{"doctor", []string{"doctor"}, true, "initApp doctor"},
		{"status", []string{"status"}, false, "initApp status"},
		{"hook", []string{"hook", "Stop"}, false, "hook"},
		{"init keeps always", []string{"init"}, true, "initApp init"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			root, dir := gateRoot(t)
			var calls []string
			e, _, stderr := fakeEnv(dir, tt.tty, true, &calls)
			e.ask = func(string) (bool, bool, error) {
				calls = append(calls, "ask")
				return true, true, nil
			}
			e.isTTY = true
			run(nil, e) // trust it always through the dialog
			e.isTTY = tt.tty
			if err := os.WriteFile(filepath.Join(dir, config.ProjectFile), []byte("# changed\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			calls = nil
			if code := run(tt.args, e); code != 0 {
				t.Errorf("exit code = %d, want 0 (stderr %q)", code, stderr.String())
			}
			if got := strings.Join(calls, " "); got != tt.wantCalls {
				t.Errorf("calls = %q, want %q", got, tt.wantCalls)
			}
			if ok, err := trust.IsTrusted(root, "other"); err != nil || !ok {
				t.Errorf("always trusted = %v (err %v), want true", ok, err)
			}
		})
	}
}

func TestRunUntrust(t *testing.T) {
	tests := []struct {
		name       string
		stored     string // trust.json entry for the root: "" none, "hash", "always", "corrupt" (the file is broken)
		tty        bool
		wantCode   int
		wantStdout string
		wantStderr string // substring; "" = stderr must be empty
	}{
		{"hash entry", "hash", true, 0, "%ROOT% is no longer trusted\n", ""},
		{"always entry", "always", true, 0, "%ROOT% is no longer trusted\n", ""},
		{"hash entry, no tty", "hash", false, 0, "%ROOT% is no longer trusted\n", ""},
		{"always entry, no tty", "always", false, 0, "%ROOT% is no longer trusted\n", ""},
		{"no entry", "", true, 0, "%ROOT% was not trusted\n", ""},
		{"no entry, no tty", "", false, 0, "%ROOT% was not trusted\n", ""},
		{"corrupt store", "corrupt", false, 1, "", "trust.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			t.Setenv("XDG_STATE_HOME", state)
			store := filepath.Join(state, "lanes", "trust.json")
			root, dir := gateRoot(t)
			hash, _ := trust.HashRoot(root)
			switch tt.stored {
			case "hash":
				if err := trust.Trust(root, hash); err != nil {
					t.Fatal(err)
				}
			case "always":
				if err := trust.TrustAlways(root); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				if err := os.MkdirAll(filepath.Dir(store), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(store, []byte("{broken"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var calls []string
			e, stdout, stderr := fakeEnv(dir, tt.tty, true, &calls)
			if code := run([]string{"--untrust"}, e); code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if len(calls) > 0 {
				t.Errorf("calls = %q, want none: no dialog, no git, no config", calls)
			}
			if want := strings.ReplaceAll(tt.wantStdout, "%ROOT%", root); stdout.String() != want {
				t.Errorf("stdout = %q, want %q", stdout.String(), want)
			}
			if tt.wantStderr == "" && stderr.Len() > 0 || !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.wantStderr)
			}
			if tt.stored == "corrupt" {
				if b, _ := os.ReadFile(store); string(b) != "{broken" {
					t.Errorf("trust.json was rewritten: %q", b)
				}
				return
			}
			if ok, err := trust.IsTrusted(root, hash); err != nil || ok {
				t.Errorf("trusted after untrust = %v (err %v), want false", ok, err)
			}
			calls = nil
			e.isTTY = true
			run(nil, e)
			if len(calls) == 0 || calls[0] != "ask" {
				t.Errorf("the next start ran %q, want the dialog first", calls)
			}
		})
	}
}

func TestRunReasksWhenProjectFileChanges(t *testing.T) {
	tests := []struct {
		name   string
		before string // .lanes.toml content when trusted; "" = none
		after  string // content at the next start; "" = none
		asks   bool
	}{
		{"unchanged", "# a\n", "# a\n", false},
		{"no file stays none", "", "", false},
		{"changed", "# a\n", "# b\n", true},
		{"added", "", "# a\n", true},
		{"removed", "# a\n", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			dir := t.TempDir()
			p := filepath.Join(dir, config.ProjectFile)
			write := func(s string) {
				os.Remove(p)
				if s != "" {
					if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
			write(tt.before)
			var calls []string
			e, _, _ := fakeEnv(dir, true, true, &calls)
			run(nil, e)
			write(tt.after)
			calls = nil
			run(nil, e)
			if asked := len(calls) > 0 && calls[0] == "ask"; asked != tt.asks {
				t.Errorf("asked = %v, want %v (calls %q)", asked, tt.asks, calls)
			}
		})
	}
}

func TestRunInitRerecordsHash(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir := t.TempDir()
	var calls []string
	e, _, _ := fakeEnv(dir, true, true, &calls)
	e.init = func() int {
		calls = append(calls, "init")
		if err := os.WriteFile(filepath.Join(dir, config.ProjectFile), []byte("# new\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return 0
	}
	if code := run([]string{"init"}, e); code != 0 {
		t.Fatalf("init exit code = %d", code)
	}
	calls = nil
	run(nil, e)
	if strings.Join(calls, " ") != "initApp tui" {
		t.Errorf("after init the next start ran %q, want no prompt", calls)
	}
}

func TestRunCorruptStore(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	store := filepath.Join(state, "lanes", "trust.json")
	if err := os.MkdirAll(filepath.Dir(store), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls []string
	e, _, stderr := fakeEnv(t.TempDir(), true, true, &calls)
	if code := run(nil, e); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if strings.Contains(strings.Join(calls, " "), "tui") {
		t.Errorf("calls = %q, the TUI must not start", calls)
	}
	if !strings.Contains(stderr.String(), "trust.json") {
		t.Errorf("stderr = %q, want the trust.json error", stderr.String())
	}
	if b, _ := os.ReadFile(store); string(b) != "{broken" {
		t.Errorf("trust.json was rewritten: %q", b)
	}
}
