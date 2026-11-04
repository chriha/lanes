package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/session"
	"github.com/chriha/lanes/internal/trust"
	"github.com/chriha/lanes/internal/update"
)

func TestResolveVersion(t *testing.T) {
	tests := []struct {
		name, ldflags, module string // module: the build info's Main.Version; "" = no build info
		want                  string
	}{
		{"dev without build info", "dev", "", "dev"},
		{"dev build from a checkout", "dev", "(devel)", "dev"},
		{"go install @v1.2.3", "dev", "v1.2.3", "v1.2.3"},
		{"go build stamps a pseudo-version", "dev", "v1.2.4-0.20261101120000-abcdef123456", "dev"},
		{"go build stamps a dirty pseudo-version", "dev", "v0.0.0-20261101120000-abcdef123456+dirty", "dev"},
		{"ldflags value wins", "v1.0.0", "v1.2.3", "v1.0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := func() (*debug.BuildInfo, bool) {
				if tt.module == "" {
					return nil, false
				}
				return &debug.BuildInfo{Main: debug.Module{Version: tt.module}}, true
			}
			if got := resolveVersion(tt.ldflags, info); got != tt.want {
				t.Errorf("resolveVersion(%q) with module %q = %q, want %q", tt.ldflags, tt.module, got, tt.want)
			}
		})
	}
}

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

// fakeEnv returns an env for dir whose steps append their name to calls; ask answers yes. The release fetch
// fails, so a test that reaches the network sees "fetch" in calls.
func fakeEnv(dir string, tty, yes bool, calls *[]string) (env, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	rec := func(s string) { *calls = append(*calls, s) }
	return env{
		wd: dir, isTTY: tty, stdout: &stdout, stderr: &stderr,
		version: "dev",
		fetch: func(context.Context) (string, error) {
			rec("fetch")
			return "", errors.New("no network in tests")
		},
		upgrade:   func() string { return update.Brew },
		updateOff: func() bool { return false },
		ask:       func(string) (bool, bool, error) { rec("ask"); return yes, false, nil },
		initApp:   func(string) { rec("initApp") },
		hook:      func(string) { rec("hook") },
		status:    func() { rec("status") },
		doctor:    func() int { rec("doctor"); return 0 },
		init: func() int {
			rec("init")
			return 0
		},
		tui: func(update.Checker, string) int { rec("tui"); return 0 },
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
		{"version trusted", []string{"version"}, true, false, true, 0, "initApp", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			t.Setenv("LANES_SESSION", "") // the test may itself run inside a lanes session
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

// An unreadable dir makes HashRoot fail, so it shows whether the gate walked the tree.
func TestRunGateWalksOnlyHashTrustedRoots(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads unreadable dirs")
	}
	tests := []struct {
		name       string
		always     bool // trust.json holds Always for the root, else the hash from before the dir became unreadable
		args       []string
		wantCode   int
		wantCalls  string
		wantStderr string // substring; "" = stderr must be empty
	}{
		{"always: tui", true, nil, 0, "initApp tui", ""},
		{"always: status", true, []string{"status"}, 0, "initApp status", ""},
		{"always: hook", true, []string{"hook", "Stop"}, 0, "hook", ""},
		{"always: version", true, []string{"version"}, 0, "initApp", ""},
		{"hash: tui walks and asks", false, nil, 0, "ask initApp tui", "permission denied"},
		{"hash: status walks and stays quiet", false, []string{"status"}, 0, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			root, dir := gateRoot(t)
			hash, err := trust.HashRoot(root)
			if err != nil {
				t.Fatal(err)
			}
			if tt.always {
				err = trust.TrustAlways(root)
			} else {
				err = trust.Trust(root, hash)
			}
			if err != nil {
				t.Fatal(err)
			}
			locked := filepath.Join(root, "locked")
			if err := os.Mkdir(locked, 0o000); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { // let TempDir remove it
				if err := os.Chmod(locked, 0o755); err != nil {
					t.Errorf("restoring %s: %v", locked, err)
				}
			})
			var calls []string
			e, _, stderr := fakeEnv(dir, true, true, &calls)
			if code := run(tt.args, e); code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if got := strings.Join(calls, " "); got != tt.wantCalls {
				t.Errorf("calls = %q, want %q", got, tt.wantCalls)
			}
			if tt.wantStderr == "" && stderr.Len() > 0 || !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.wantStderr)
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

// writeUpdateCache stores a successful release check that found latest, as the TUI's check would.
func writeUpdateCache(t *testing.T, latest string) string {
	t.Helper()
	p := filepath.Join(app.CacheDir(), "update.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	b := `{"checked_at":"` + time.Now().Add(-time.Hour).Format(time.RFC3339) + `","latest":"` + latest + `"}`
	if err := os.WriteFile(p, []byte(b), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunVersionUpdateHint(t *testing.T) {
	tests := []struct {
		name    string
		version string
		cached  string // latest release in update.json; "" = no cache file
		trusted bool
		off     bool // settings.no_update_check
		want    string
	}{
		{"newer cached", "v1.0.0", "v1.1.0", true, false, "lanes v1.0.0\nv1.1.0 available · brew upgrade lanes\n"},
		{"same cached", "v1.1.0", "v1.1.0", true, false, "lanes v1.1.0\n"},
		{"older cached", "v1.2.0", "v1.1.0", true, false, "lanes v1.2.0\n"},
		{"no cache", "v1.0.0", "", true, false, "lanes v1.0.0\n"},
		{"prerelease of the cached release", "v1.1.0-rc1", "v1.1.0", true, false, "lanes v1.1.0-rc1\nv1.1.0 available · brew upgrade lanes\n"},
		{"no_update_check", "v1.0.0", "v1.1.0", true, true, "lanes v1.0.0\n"},
		{"dev build", "dev", "v1.1.0", true, false, "lanes dev\n"},
		{"untrusted: no project cache", "v1.0.0", "v1.1.0", false, false, "lanes v1.0.0\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			root, dir := gateRoot(t)
			if tt.trusted {
				hash, _ := trust.HashRoot(root)
				if err := trust.Trust(root, hash); err != nil {
					t.Fatal(err)
				}
			}
			var before []byte
			if tt.cached != "" {
				before, _ = os.ReadFile(writeUpdateCache(t, tt.cached))
			}
			for _, args := range [][]string{{"version"}, {"--version"}} {
				var calls []string
				e, stdout, stderr := fakeEnv(dir, false, true, &calls)
				e.version = tt.version
				e.updateOff = func() bool { return tt.off }
				if code := run(args, e); code != 0 {
					t.Errorf("%s: exit code = %d, want 0", args[0], code)
				}
				if got := stdout.String(); got != tt.want {
					t.Errorf("%s: stdout = %q, want %q", args[0], got, tt.want)
				}
				if stderr.Len() > 0 {
					t.Errorf("%s: stderr = %q, want empty", args[0], stderr.String())
				}
				if strings.Contains(strings.Join(calls, " "), "fetch") {
					t.Errorf("%s: calls = %q, version must never ask the network", args[0], calls)
				}
			}
			after, _ := os.ReadFile(filepath.Join(app.CacheDir(), "update.json"))
			if !bytes.Equal(before, after) {
				t.Errorf("update.json changed: %q -> %q", before, after)
			}
		})
	}
}

func TestRunChecksUpdatesOnlyInTUI(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"hook", []string{"hook", "Stop"}},
		{"status", []string{"status"}},
		{"untrust", []string{"--untrust"}},
		{"init", []string{"init"}},
		{"doctor", []string{"doctor"}},
		{"tui", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			root, dir := gateRoot(t)
			hash, _ := trust.HashRoot(root)
			if err := trust.Trust(root, hash); err != nil {
				t.Fatal(err)
			}
			var calls []string
			e, _, _ := fakeEnv(dir, true, true, &calls)
			e.version = "v1.0.0"
			var got *update.Checker
			var upgrade string
			e.tui = func(c update.Checker, u string) int {
				calls = append(calls, "tui")
				got, upgrade = &c, u
				return 0
			}
			run(tt.args, e)
			if strings.Contains(strings.Join(calls, " "), "fetch") {
				t.Errorf("calls = %q, want no release check", calls)
			}
			if _, err := os.Stat(filepath.Join(app.CacheDir(), "update.json")); !os.IsNotExist(err) {
				t.Errorf("update.json exists (err %v), want no release check", err)
			}
			if tt.args != nil {
				if got != nil {
					t.Errorf("the TUI got a checker for %q", tt.args)
				}
				return
			}
			if got == nil {
				t.Fatal("the TUI got no checker")
			}
			if got.Current != "v1.0.0" || upgrade != update.Brew {
				t.Errorf("checker for %q with %q, want v1.0.0 with %q", got.Current, upgrade, update.Brew)
			}
			if want := filepath.Join(app.CacheDir(), "update.json"); got.CachePath != want {
				t.Errorf("CachePath = %q, want %q", got.CachePath, want)
			}
			if _, err := got.Fetch(context.Background()); err == nil || !strings.Contains(strings.Join(calls, " "), "fetch") {
				t.Errorf("the checker does not fetch through env.fetch (calls %q)", calls)
			}
		})
	}
}

// A Claude session lanes starts in a git worktree of a trusted project runs in a root of its own, which is not
// trusted. Its hooks still have to record the transcript, or the agent view cannot open for the session.
func TestRunHookInWorktreeSession(t *testing.T) {
	const payload = `{"session_id":"abc","transcript_path":"/t/abc.jsonl"}`
	tests := []struct {
		name       string
		inSession  bool // LANES_SESSION and LANES_STATE_DIR set, as lanes sets them in the sessions it starts
		trustedWT  bool
		wantRecord bool
	}{
		{"lanes session, worktree not trusted", true, false, true},
		{"lanes session, worktree trusted", true, true, true},
		{"no lanes session, worktree not trusted", false, false, false},
		{"no lanes session, worktree trusted", false, true, false}, // the hook has no session to record for
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			if err := os.MkdirAll(filepath.Join(app.StateDir(), "sessions"), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("LANES_SESSION", "")
			t.Setenv("LANES_STATE_DIR", "")
			if tt.inSession {
				t.Setenv("LANES_SESSION", "fix~1")
				t.Setenv("LANES_STATE_DIR", app.StateDir())
			}
			wt := t.TempDir()
			if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: /elsewhere/.git/worktrees/x\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if tt.trustedWT {
				if err := trust.TrustAlways(wt); err != nil {
					t.Fatal(err)
				}
			}
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.WriteString(payload); err != nil {
				t.Fatal(err)
			}
			w.Close()
			stdin := os.Stdin
			os.Stdin = r
			t.Cleanup(func() { os.Stdin = stdin })

			var calls []string
			e, _, _ := fakeEnv(wt, false, false, &calls)
			e.hook = session.RunHook
			if code := run([]string{"hook", "UserPromptSubmit"}, e); code != 0 {
				t.Fatalf("exit code = %d, want 0", code)
			}
			got := session.ClaudeTranscript("fix~1")
			if want := "/t/abc.jsonl"; tt.wantRecord && got != want || !tt.wantRecord && got != "" {
				t.Errorf("transcript = %q, want recorded %v", got, tt.wantRecord)
			}
		})
	}
}
