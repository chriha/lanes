package trust

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

// ageDirs sets the mtime of every directory under root to at, so only what a test changes afterwards looks new.
func ageDirs(t testing.TB, root string, at time.Time) {
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

// walkFixture builds a tree under a new root, with HOME outside it, whose walked directories are fixtureDirs, all
// with an mtime an hour before the returned time.
func walkFixture(t *testing.T) (root string, written time.Time) {
	t.Helper()
	t.Setenv("HOME", t.TempDir()) // no .lanes.toml above the root
	root = filepath.Join(t.TempDir(), "root")
	mustMkdir(t, filepath.Join(root, "a", "b"))
	mustMkdir(t, filepath.Join(root, "c"))
	mustMkdir(t, filepath.Join(root, ".git"))
	mustMkdir(t, filepath.Join(root, "node_modules", "x"))
	mustWrite(t, filepath.Join(root, ".lanes.toml"), "root = 1\n")
	mustWrite(t, filepath.Join(root, "a", ".lanes.toml"), "a = 1\n")
	mustWrite(t, filepath.Join(root, ".git", ".lanes.toml"), "ignored = 1\n")
	mustWrite(t, filepath.Join(root, "node_modules", "x", ".lanes.toml"), "ignored = 1\n")
	written = time.Now()
	ageDirs(t, root, written.Add(-time.Hour))
	return root, written
}

// fixtureDirs is every directory a walk of walkFixture's tree reads, sorted.
var fixtureDirs = []string{".", "a", "a/b", "c"}

func TestHashRootWithCache(t *testing.T) {
	all := fixtureDirs
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
			root, written := walkFixture(t)
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

// cachedHash is hashRootCached at now, returning the sorted directories it read.
func cachedHash(t *testing.T, root string, now time.Time) (string, []string) {
	t.Helper()
	var read []string
	h, err := hashRootCached(root, now, recordingReadDir(root, &read))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(read)
	return h, read
}

// uncachedHash is the hash of a full walk of root.
func uncachedHash(t *testing.T, root string) string {
	t.Helper()
	h, _, err := hashRootWith(root, nil, time.Now(), os.ReadDir)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// editCache rewrites root's cache file through edit.
func editCache(t *testing.T, root string, edit func(c *walkCache)) {
	t.Helper()
	b, err := os.ReadFile(walkCachePath(root))
	if err != nil {
		t.Fatal(err)
	}
	var c walkCache
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	edit(&c)
	if b, err = json.Marshal(c); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, walkCachePath(root), string(b))
}

// hideConfigs makes c claim there is no .lanes.toml anywhere, so a walk that used it would hash nothing.
func hideConfigs(c *walkCache) {
	for rel, d := range c.Dirs {
		d.Config = false
		c.Dirs[rel] = d
	}
}

func TestHashRootPersistsCache(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root, written := walkFixture(t)

	first, read := cachedHash(t, root, written)
	if !reflect.DeepEqual(read, fixtureDirs) {
		t.Fatalf("first start read %v, want %v", read, fixtureDirs)
	}
	if _, err := os.Stat(walkCachePath(root)); err != nil {
		t.Fatalf("no cache file after the first start: %v", err)
	}

	second, read := cachedHash(t, root, written.Add(time.Minute))
	if read != nil {
		t.Errorf("start of an unchanged tree read %v, want none", read)
	}
	if second != first {
		t.Errorf("hash of an unchanged tree changed: %q -> %q", first, second)
	}

	mustWrite(t, filepath.Join(root, "c", ".lanes.toml"), "c = 1\n")
	third, read := cachedHash(t, root, written.Add(2*time.Minute))
	if !reflect.DeepEqual(read, []string{"c"}) {
		t.Errorf("start after adding c/.lanes.toml read %v, want [c]", read)
	}
	if want := uncachedHash(t, root); third != want || third == first {
		t.Errorf("hash after adding c/.lanes.toml = %q, want the full walk's %q (before: %q)", third, want, first)
	}
}

func TestHashRootIgnoresBadCache(t *testing.T) {
	tests := []struct {
		name  string
		spoil func(t *testing.T, root string)
	}{
		{name: "missing", spoil: func(t *testing.T, root string) {
			if err := os.Remove(walkCachePath(root)); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "corrupt", spoil: func(t *testing.T, root string) {
			mustWrite(t, walkCachePath(root), "{not json")
		}},
		{name: "JSON null", spoil: func(t *testing.T, root string) {
			mustWrite(t, walkCachePath(root), "null")
		}},
		{name: "other version", spoil: func(t *testing.T, root string) {
			editCache(t, root, func(c *walkCache) { hideConfigs(c); c.Version++ })
		}},
		{name: "other root", spoil: func(t *testing.T, root string) {
			editCache(t, root, func(c *walkCache) { hideConfigs(c); c.Root += "x" })
		}},
		{name: "child directory outside the root", spoil: func(t *testing.T, root string) {
			outside := filepath.Join(filepath.Dir(root), "outside")
			mustMkdir(t, outside)
			mustWrite(t, filepath.Join(outside, ".lanes.toml"), "outside = 1\n")
			ageDirs(t, outside, time.Now().Add(-time.Hour))
			editCache(t, root, func(c *walkCache) {
				d := c.Dirs["."]
				d.Dirs = append(d.Dirs, "../outside")
				c.Dirs["."] = d
				c.Dirs["../outside"] = walkDir{ModTime: time.Now().Add(-time.Hour).UnixNano(), Config: true}
			})
		}},
		{name: "child directory that does not exist", spoil: func(t *testing.T, root string) {
			editCache(t, root, func(c *walkCache) {
				d := c.Dirs["."]
				d.Dirs = append(d.Dirs, "gone")
				c.Dirs["."] = d
			})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			root, written := walkFixture(t)
			cachedHash(t, root, written)
			tt.spoil(t, root)

			got, read := cachedHash(t, root, written.Add(time.Minute))
			if want := uncachedHash(t, root); got != want {
				t.Errorf("hash with a bad cache = %q, want the full walk's %q", got, want)
			}
			if !reflect.DeepEqual(read, fixtureDirs) {
				t.Errorf("start with a bad cache read %v, want the full walk %v", read, fixtureDirs)
			}
			if _, read := cachedHash(t, root, written.Add(2*time.Minute)); read != nil {
				t.Errorf("start after the fallback read %v, want none: the full walk should rewrite the cache", read)
			}
		})
	}
}

func TestHashRootIgnoresFailedCacheWrite(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	mustWrite(t, state, "a file, so the cache dir cannot be created\n")
	t.Setenv("XDG_STATE_HOME", state)
	root, written := walkFixture(t)

	got, _ := cachedHash(t, root, written) // fails the test on an error
	if want := uncachedHash(t, root); got != want {
		t.Errorf("hash with an unwritable state dir = %q, want the full walk's %q", got, want)
	}
}
