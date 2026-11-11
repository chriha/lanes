package trust

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

// ageDirs sets the mtime of every directory under root to at, so only what a test changes afterwards looks new.
func ageDirs(t *testing.T, root string, at time.Time) {
	t.Helper()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		return os.Chtimes(p, at, at)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// setMtime sets the mtime of the directory rel under root.
func setMtime(t *testing.T, root, rel string, at time.Time) {
	t.Helper()
	if err := os.Chtimes(filepath.Join(root, rel), at, at); err != nil {
		t.Fatal(err)
	}
}

// recordingReadDir is os.ReadDir that records which directories under root it read, as slash paths relative to root.
func recordingReadDir(root string, read *[]string) readDirFunc {
	return func(p string) ([]fs.DirEntry, error) {
		rel, _ := filepath.Rel(root, p)
		*read = append(*read, filepath.ToSlash(rel))
		return os.ReadDir(p)
	}
}

func TestHashRootWithCache(t *testing.T) {
	all := []string{".", "a", "a/b", "c"} // every walked directory of the fixture
	tests := []struct {
		name        string
		prep        func(t *testing.T, root string, written time.Time) // before the walk that writes the cache
		change      func(t *testing.T, root string, written time.Time) // between the two walks
		wantRead    []string                                           // directories the cached walk reads
		wantChanged bool
	}{
		{
			name:     "unchanged tree reads no directory",
			wantRead: nil,
		},
		{
			name: "file edited in place",
			change: func(t *testing.T, root string, written time.Time) {
				mustWrite(t, filepath.Join(root, "a", ".lanes.toml"), "a = 2\n")
				setMtime(t, root, "a", written.Add(-time.Hour))
			},
			wantRead:    nil,
			wantChanged: true,
		},
		{
			name: "file added in an existing directory",
			change: func(t *testing.T, root string, _ time.Time) {
				mustWrite(t, filepath.Join(root, "c", ".lanes.toml"), "c = 1\n")
			},
			wantRead:    []string{"c"},
			wantChanged: true,
		},
		{
			name: "file added in a new directory",
			change: func(t *testing.T, root string, _ time.Time) {
				mustMkdir(t, filepath.Join(root, "c", "d"))
				mustWrite(t, filepath.Join(root, "c", "d", ".lanes.toml"), "d = 1\n")
			},
			wantRead:    []string{"c", "c/d"},
			wantChanged: true,
		},
		{
			name: "file removed",
			change: func(t *testing.T, root string, _ time.Time) {
				if err := os.Remove(filepath.Join(root, "a", ".lanes.toml")); err != nil {
					t.Fatal(err)
				}
			},
			wantRead:    []string{"a"},
			wantChanged: true,
		},
		{
			name: "directory without a file removed",
			change: func(t *testing.T, root string, _ time.Time) {
				if err := os.Remove(filepath.Join(root, "a", "b")); err != nil {
					t.Fatal(err)
				}
			},
			wantRead: []string{"a"},
		},
		{
			name: "directory changed within the racy window is re-read",
			prep: func(t *testing.T, root string, written time.Time) {
				setMtime(t, root, "c", written.Add(-time.Second))
			},
			wantRead: []string{"c"},
		},
		{
			name: "file added in the racy window without an mtime change",
			prep: func(t *testing.T, root string, written time.Time) {
				setMtime(t, root, "c", written.Add(-time.Second))
			},
			change: func(t *testing.T, root string, written time.Time) {
				mustWrite(t, filepath.Join(root, "c", ".lanes.toml"), "c = 1\n")
				setMtime(t, root, "c", written.Add(-time.Second))
			},
			wantRead:    []string{"c"},
			wantChanged: true,
		},
		{
			name: "cached directory gone without an mtime change falls back to the full walk",
			change: func(t *testing.T, root string, written time.Time) {
				if err := os.Remove(filepath.Join(root, "a", "b")); err != nil {
					t.Fatal(err)
				}
				setMtime(t, root, "a", written.Add(-time.Hour))
			},
			wantRead: []string{".", "a", "c"},
		},
		{
			name: "cached file gone without an mtime change falls back to the full walk",
			change: func(t *testing.T, root string, written time.Time) {
				if err := os.Remove(filepath.Join(root, "a", ".lanes.toml")); err != nil {
					t.Fatal(err)
				}
				setMtime(t, root, "a", written.Add(-time.Hour))
			},
			wantRead:    all,
			wantChanged: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir()) // no .lanes.toml above the root
			root := t.TempDir()
			mustMkdir(t, filepath.Join(root, "a", "b"))
			mustMkdir(t, filepath.Join(root, "c"))
			mustMkdir(t, filepath.Join(root, ".git"))
			mustMkdir(t, filepath.Join(root, "node_modules", "x"))
			mustWrite(t, filepath.Join(root, ".lanes.toml"), "root = 1\n")
			mustWrite(t, filepath.Join(root, "a", ".lanes.toml"), "a = 1\n")
			mustWrite(t, filepath.Join(root, ".git", ".lanes.toml"), "ignored = 1\n")
			mustWrite(t, filepath.Join(root, "node_modules", "x", ".lanes.toml"), "ignored = 1\n")
			written := time.Now()
			ageDirs(t, root, written.Add(-time.Hour))
			if tt.prep != nil {
				tt.prep(t, root, written)
			}

			var read []string
			before, cache, err := hashRootWith(root, nil, written, recordingReadDir(root, &read))
			if err != nil {
				t.Fatal(err)
			}
			sort.Strings(read)
			if !reflect.DeepEqual(read, all) {
				t.Fatalf("walk without a cache read %v, want %v", read, all)
			}

			if tt.change != nil {
				tt.change(t, root, written)
			}
			read = nil
			after, next, err := hashRootWith(root, cache, written.Add(time.Minute), recordingReadDir(root, &read))
			if err != nil {
				t.Fatal(err)
			}
			sort.Strings(read)
			if !reflect.DeepEqual(read, tt.wantRead) {
				t.Errorf("cached walk read %v, want %v", read, tt.wantRead)
			}
			if (after != before) != tt.wantChanged {
				t.Errorf("hash changed = %v, want %v", after != before, tt.wantChanged)
			}

			full, fresh, err := hashRootWith(root, nil, written.Add(time.Minute), os.ReadDir)
			if err != nil {
				t.Fatal(err)
			}
			if after != full {
				t.Errorf("cached walk hash %q != uncached walk hash %q", after, full)
			}
			if !reflect.DeepEqual(next.Dirs, fresh.Dirs) {
				t.Errorf("cache after the cached walk %v, want the uncached walk's %v", next.Dirs, fresh.Dirs)
			}
		})
	}
}

func TestHashRootWithMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "gone")
	cache := &walkCache{Dirs: map[string]walkDir{".": {Config: true}}}
	for _, prev := range []*walkCache{nil, cache} {
		if h, _, err := hashRootWith(root, prev, time.Now(), os.ReadDir); h != "" || err == nil {
			t.Errorf("hashRootWith(missing root, cache %v) = %q, %v; want \"\", error", prev != nil, h, err)
		}
	}
}
