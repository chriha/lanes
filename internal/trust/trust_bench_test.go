package trust

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// benchTree makes a large project tree under root: 1,000 source dirs of 20 files each, a .lanes.toml in some of
// them, and a .git and node_modules the walk skips. Every directory gets an old mtime, as in a tree not touched for
// a while.
func benchTree(b *testing.B, root string) {
	b.Helper()
	write := func(p, body string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	write(filepath.Join(root, ".lanes.toml"), "# project\n")
	for d := 0; d < 50; d++ {
		for s := 0; s < 20; s++ {
			dir := filepath.Join(root, "pkg", fmt.Sprintf("d%02d", d), fmt.Sprintf("s%02d", s))
			for f := 0; f < 20; f++ {
				write(filepath.Join(dir, fmt.Sprintf("f%02d.go", f)), "package s\n")
			}
			if s == 0 && d%10 == 0 {
				write(filepath.Join(dir, ".lanes.toml"), "# nested\n")
			}
		}
	}
	for _, skipped := range []string{".git", "node_modules"} {
		for f := 0; f < 100; f++ {
			write(filepath.Join(root, skipped, fmt.Sprintf("x%02d", f%10), fmt.Sprintf("y%03d", f)), "x\n")
		}
	}
	ageDirs(b, root, time.Now().Add(-time.Hour))
}

// BenchmarkHashRoot compares the full walk HashRoot did before the cache with HashRoot on an unchanged tree whose
// cache file a previous start wrote (load, stat every directory, read every .lanes.toml, save).
func BenchmarkHashRoot(b *testing.B) {
	b.Setenv("HOME", b.TempDir())
	b.Setenv("XDG_STATE_HOME", b.TempDir())
	root := filepath.Join(b.TempDir(), "root")
	benchTree(b, root)
	want, _, err := hashRootWith(root, nil, time.Now(), os.ReadDir)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("uncached", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if h, _, err := hashRootWith(root, nil, time.Now(), os.ReadDir); err != nil || h != want {
				b.Fatalf("hash = %q, %v; want %q", h, err, want)
			}
		}
	})
	b.Run("cached", func(b *testing.B) {
		if _, err := HashRoot(root); err != nil { // the previous start that writes the cache
			b.Fatal(err)
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if h, err := HashRoot(root); err != nil || h != want {
				b.Fatalf("hash = %q, %v; want %q", h, err, want)
			}
		}
	})
}
