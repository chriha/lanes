package jsonfile

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// existing describes the file at the target path before Write.
type existing struct {
	body string
	mode os.FileMode
}

func TestWrite(t *testing.T) {
	want := map[string]string{"a": "1"}
	tests := []struct {
		name          string
		before        *existing // nil: no file yet
		backupCorrupt bool
		wantBak       string // "" when no .bak may exist
	}{
		{name: "new file in a new dir", before: nil, backupCorrupt: true},
		{name: "healthy file is replaced, not moved", before: &existing{`{"b":"2"}`, 0o600}, backupCorrupt: true},
		{name: "wider mode is tightened", before: &existing{`{"b":"2"}`, 0o644}, backupCorrupt: false},
		{name: "invalid JSON is moved to .bak", before: &existing{`{not json`, 0o600}, backupCorrupt: true, wantBak: `{not json`},
		{name: "wrong shape is moved to .bak", before: &existing{`[1,2]`, 0o600}, backupCorrupt: true, wantBak: `[1,2]`},
		{name: "unreadable file is not moved", before: &existing{`{not json`, 0o000}, backupCorrupt: true},
		{name: "corrupt file without the flag is not moved", before: &existing{`{not json`, 0o600}, backupCorrupt: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "state", "project")
			path := filepath.Join(dir, "store.json")
			if tt.before != nil {
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(tt.before.body), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, tt.before.mode); err != nil {
					t.Fatal(err)
				}
				if tt.before.mode == 0o000 {
					if _, err := os.ReadFile(path); err == nil {
						t.Skip("file mode 000 is still readable (running as root)")
					}
				}
			}

			if err := Write(path, want, tt.backupCorrupt); err != nil {
				t.Fatalf("Write: %v", err)
			}

			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]string
			if err := json.Unmarshal(b, &got); err != nil || len(got) != 1 || got["a"] != "1" {
				t.Errorf("file holds %q (%v), want %v", b, err, want)
			}
			if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
				t.Errorf("mode = %v (%v), want 0600", fi.Mode().Perm(), err)
			}
			if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != 0o700 {
				t.Errorf("dir mode = %v (%v), want 0700 when Write created it", fi.Mode().Perm(), err)
			}
			bak, err := os.ReadFile(path + ".bak")
			switch {
			case tt.wantBak == "" && !errors.Is(err, fs.ErrNotExist):
				t.Errorf(".bak exists (%q, %v), want none", bak, err)
			case tt.wantBak != "" && string(bak) != tt.wantBak:
				t.Errorf(".bak = %q (%v), want %q", bak, err, tt.wantBak)
			}
			assertNoTemp(t, dir)
		})
	}
}

func TestWriteFailureLeavesNoTempFile(t *testing.T) {
	tests := []struct {
		name  string
		value any
		setup func(t *testing.T, path string)
	}{
		{name: "value does not marshal", value: map[string]any{"c": make(chan int)}},
		{name: "rename over a non-empty dir fails", value: map[string]string{"a": "1"}, setup: func(t *testing.T, path string) {
			if err := os.MkdirAll(filepath.Join(path, "child"), 0o700); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "store.json")
			if tt.setup != nil {
				tt.setup(t, path)
			}
			if err := Write(path, tt.value, true); err == nil {
				t.Fatal("Write succeeded, want an error")
			}
			assertNoTemp(t, dir)
		})
	}
}

// assertNoTemp fails when dir holds anything besides store.json and store.json.bak.
func assertNoTemp(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if n := e.Name(); n != "store.json" && n != "store.json.bak" {
			t.Errorf("leftover file %s", n)
		}
	}
}
