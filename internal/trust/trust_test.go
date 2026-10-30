package trust

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoot(t *testing.T) {
	tests := []struct {
		name string
		git  string // "dir", "file" or "" for no .git at top
		sub  string
		want string // "top" or "sub"
	}{
		{"git dir at top", "dir", "", "top"},
		{"nested below git dir", "dir", "a/b", "top"},
		{"git file (worktree)", "file", "a", "top"},
		{"no git", "", "a/b", "sub"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			top := t.TempDir()
			switch tt.git {
			case "dir":
				mustMkdir(t, filepath.Join(top, ".git"))
			case "file":
				mustWrite(t, filepath.Join(top, ".git"), "gitdir: /elsewhere\n")
			}
			sub := filepath.Join(top, tt.sub)
			mustMkdir(t, sub)
			want := map[string]string{"top": top, "sub": sub}[tt.want]
			if got := Root(sub); got != want {
				t.Errorf("Root(%q) = %q, want %q", sub, got, want)
			}
		})
	}
}

func TestHash(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.toml"), filepath.Join(dir, "b.toml")
	mustWrite(t, a, "x = 1\n")
	mustWrite(t, b, "x = 2\n")

	ha, err := Hash(a)
	if err != nil || len(ha) != 64 {
		t.Fatalf("Hash(a) = %q, %v; want 64 hex chars", ha, err)
	}
	if again, _ := Hash(a); again != ha {
		t.Errorf("Hash not stable: %q vs %q", again, ha)
	}
	if hb, _ := Hash(b); hb == ha {
		t.Errorf("different content, same hash %q", ha)
	}
	for _, p := range []string{"", filepath.Join(dir, "missing.toml")} {
		if h, err := Hash(p); h != "" || err != nil {
			t.Errorf("Hash(%q) = %q, %v; want \"\", nil", p, h, err)
		}
	}
}

func TestTrustRoundTrip(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if ok, err := IsTrusted("/p", "h1"); ok || err != nil {
		t.Fatalf("no trust.json: IsTrusted = %v, %v; want false, nil", ok, err)
	}
	if err := Trust("/p", "h1"); err != nil {
		t.Fatal(err)
	}
	if err := Trust("/q", ""); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		root, hash string
		want       bool
	}{
		{"/p", "h1", true},
		{"/p", "h2", false}, // .lanes.toml changed
		{"/p", "", false},   // .lanes.toml removed
		{"/q", "", true},    // trusted without .lanes.toml
		{"/q", "h1", false}, // .lanes.toml added
		{"/r", "", false},
	}
	for _, tt := range tests {
		if ok, err := IsTrusted(tt.root, tt.hash); ok != tt.want || err != nil {
			t.Errorf("IsTrusted(%q, %q) = %v, %v; want %v, nil", tt.root, tt.hash, ok, err, tt.want)
		}
	}

	fi, err := os.Stat(path())
	if err != nil {
		t.Fatal(err)
	}
	if mode := fi.Mode().Perm(); mode != 0o600 {
		t.Errorf("trust.json mode = %v, want 0600", mode)
	}
}

func TestCorruptStore(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	mustMkdir(t, filepath.Dir(path()))
	mustWrite(t, path(), "{not json")

	if ok, err := IsTrusted("/p", ""); ok || err == nil {
		t.Errorf("IsTrusted on corrupt file = %v, %v; want false, error", ok, err)
	}
	if err := Trust("/p", ""); err == nil {
		t.Error("Trust on corrupt file: want error")
	}
	if b, _ := os.ReadFile(path()); string(b) != "{not json" {
		t.Errorf("corrupt trust.json was overwritten: %q", b)
	}
}

func TestNullStore(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	mustMkdir(t, filepath.Dir(path()))
	mustWrite(t, path(), "null")

	if err := Trust("/p", "h"); err != nil {
		t.Fatal(err)
	}
	if ok, err := IsTrusted("/p", "h"); !ok || err != nil {
		t.Errorf("IsTrusted = %v, %v; want true, nil", ok, err)
	}
}

func TestRootRelative(t *testing.T) {
	top := t.TempDir()
	mustMkdir(t, filepath.Join(top, ".git"))
	mustMkdir(t, filepath.Join(top, "a"))
	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.Chdir(filepath.Join(top, "a")); err != nil {
		t.Fatal(err)
	}

	want, _ := filepath.EvalSymlinks(top)
	if got, _ := filepath.EvalSymlinks(Root(".")); got != want {
		t.Errorf("Root(\".\") = %q, want %q", got, want)
	}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}
