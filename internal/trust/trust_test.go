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

func TestHashRootIndependentOfCwd(t *testing.T) {
	top := t.TempDir()
	mustMkdir(t, filepath.Join(top, ".git"))
	mustMkdir(t, filepath.Join(top, "sub", "deep"))
	mustWrite(t, filepath.Join(top, ".lanes.toml"), "a = 1\n")
	mustWrite(t, filepath.Join(top, "sub", ".lanes.toml"), "b = 2\n")
	mustWrite(t, filepath.Join(top, ".git", ".lanes.toml"), "ignored = true\n")

	h, err := HashRoot(Root(filepath.Join(top, "sub", "deep")))
	if err != nil || len(h) != 64 {
		t.Fatalf("HashRoot = %q, %v; want 64 hex chars", h, err)
	}
	if again, _ := HashRoot(Root(top)); again != h {
		t.Errorf("hash from root %q != hash from subdirectory %q", again, h)
	}

	mustWrite(t, filepath.Join(top, "sub", ".lanes.toml"), "b = 3\n")
	if changed, _ := HashRoot(top); changed == h {
		t.Error("changed subdirectory .lanes.toml kept the same hash")
	}
	mustWrite(t, filepath.Join(top, "sub", ".lanes.toml"), "b = 2\n")
	mustWrite(t, filepath.Join(top, ".lanes.toml"), "a = 2\n")
	if changed, _ := HashRoot(top); changed == h {
		t.Error("changed root .lanes.toml kept the same hash")
	}

	if h, err := HashRoot(t.TempDir()); h != "" || err != nil {
		t.Errorf("HashRoot without files = %q, %v; want \"\", nil", h, err)
	}
}

func TestTrustFileIsPrivate(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := Trust("/r", "h"); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]os.FileMode{filepath.Dir(path()): 0o700, path(): 0o600} {
		if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s = %v, %v; want %o", p, fi, err, want)
		}
	}
}

func TestAlwaysTrusted(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := TrustAlways("/a"); err != nil {
		t.Fatal(err)
	}
	if err := Trust("/p", "h1"); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, root, hash string
		want             bool
	}{
		{"always, original hash irrelevant", "/a", "h1", true},
		{"always, changed hash", "/a", "h2", true},
		{"always, no .lanes.toml", "/a", "", true},
		{"always, hash equal to the sentinel", "/a", Always, true},
		{"hash entry, other hash", "/p", "h2", false},
		{"hash entry, sentinel as hash", "/p", Always, false},
		{"no entry, sentinel as hash", "/r", Always, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if ok, err := IsTrusted(tt.root, tt.hash); ok != tt.want || err != nil {
				t.Errorf("IsTrusted(%q, %q) = %v, %v; want %v, nil", tt.root, tt.hash, ok, err, tt.want)
			}
		})
	}
}

func TestTrustKeepsAlways(t *testing.T) {
	tests := []struct {
		name  string
		first func() error
		hash  string
		want  string // stored value after Trust(root, hash)
	}{
		{"always is not downgraded", func() error { return TrustAlways("/p") }, "h2", Always},
		{"always is not downgraded by an empty hash", func() error { return TrustAlways("/p") }, "", Always},
		{"hash is replaced", func() error { return Trust("/p", "h1") }, "h2", "h2"},
		{"no entry gets the hash", func() error { return nil }, "h2", "h2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			if err := tt.first(); err != nil {
				t.Fatal(err)
			}
			if err := Trust("/p", tt.hash); err != nil {
				t.Fatal(err)
			}
			m, err := load()
			if err != nil {
				t.Fatal(err)
			}
			if got := m["/p"]; got != tt.want {
				t.Errorf("stored %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTrustAlwaysReplacesHash(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := Trust("/p", "h1"); err != nil {
		t.Fatal(err)
	}
	if err := TrustAlways("/p"); err != nil {
		t.Fatal(err)
	}
	if ok, err := IsTrusted("/p", "h2"); !ok || err != nil {
		t.Errorf("IsTrusted after TrustAlways = %v, %v; want true, nil", ok, err)
	}
	if fi, err := os.Stat(path()); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("trust.json = %v, %v; want mode 0600", fi, err)
	}
}

func TestUntrust(t *testing.T) {
	tests := []struct {
		name        string
		seed        func() error
		wantRemoved bool
	}{
		{"hash entry", func() error { return Trust("/p", "h1") }, true},
		{"always entry", func() error { return TrustAlways("/p") }, true},
		{"empty-hash entry", func() error { return Trust("/p", "") }, true},
		{"no entry", func() error { return Trust("/other", "h1") }, false},
		{"no trust.json", func() error { return nil }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			if err := tt.seed(); err != nil {
				t.Fatal(err)
			}
			before, err := load()
			if err != nil {
				t.Fatal(err)
			}

			removed, err := Untrust("/p")
			if err != nil || removed != tt.wantRemoved {
				t.Fatalf("Untrust = %v, %v; want %v, nil", removed, err, tt.wantRemoved)
			}

			after, err := load()
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := after["/p"]; ok {
				t.Errorf("entry for /p still present: %v", after)
			}
			for root, v := range before {
				if root != "/p" && after[root] != v {
					t.Errorf("entry %q = %q after Untrust, want %q kept", root, after[root], v)
				}
			}
			for _, h := range []string{"h1", "", Always} {
				if ok, _ := IsTrusted("/p", h); ok {
					t.Errorf("IsTrusted(/p, %q) after Untrust = true", h)
				}
			}
		})
	}
}

func TestUntrustCorruptStore(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	mustMkdir(t, filepath.Dir(path()))
	mustWrite(t, path(), "{not json")

	if removed, err := Untrust("/p"); removed || err == nil {
		t.Errorf("Untrust on corrupt file = %v, %v; want false, error", removed, err)
	}
	if err := TrustAlways("/p"); err == nil {
		t.Error("TrustAlways on corrupt file: want error")
	}
	if b, _ := os.ReadFile(path()); string(b) != "{not json" {
		t.Errorf("corrupt trust.json was overwritten: %q", b)
	}
}
