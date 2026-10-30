// Package trust records which projects the user trusts lanes to run in. A project's .lanes.toml can run shell
// commands, so lanes asks before it touches an untrusted project.
package trust

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/chriha/lanes/internal/app"
)

// Root is the project root of dir: the nearest directory at or above it containing .git (a directory, or a file in
// worktrees), else dir itself. It never runs git, since git would already read the repo's own config.
func Root(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs // a relative dir would stop walking at "."
	}
	for d := dir; ; {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return dir
		}
		d = parent
	}
}

// Hash is the hex SHA-256 of the file at path, or "" when there is no file.
func Hash(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// HashRoot is the hex SHA-256 over every .lanes.toml under root (relative path and content, in path order), or ""
// when there is none. Unlike Hash of the cwd's nearest file it does not depend on where lanes was started, so one
// trust entry covers every cwd in the root, and a change to any of those files still changes the hash.
func HashRoot(root string) (string, error) {
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); p != root && (n == ".git" || n == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == ".lanes.toml" && d.Type().IsRegular() {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", nil
	}
	sort.Strings(files) // WalkDir is lexical per directory, not across a flat path list
	h := sha256.New()
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		rel, _ := filepath.Rel(root, p)
		fmt.Fprintf(h, "%s\x00%d\x00", filepath.ToSlash(rel), len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// path is global, not per project scope: trust is looked up before the scope exists.
func path() string { return filepath.Join(app.StateRoot(), "trust.json") }

// load reads root → hash. A missing file trusts nothing; a corrupt one is an error.
func load() (map[string]string, error) {
	m := map[string]string{}
	b, err := os.ReadFile(path())
	if errors.Is(err, fs.ErrNotExist) {
		return m, nil
	}
	if err == nil {
		err = json.Unmarshal(b, &m)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path(), err)
	}
	if m == nil { // the file held JSON null
		m = map[string]string{}
	}
	return m, nil
}

// IsTrusted reports whether root was trusted with this .lanes.toml hash. On error it is false.
func IsTrusted(root, hash string) (bool, error) {
	m, err := load()
	if err != nil {
		return false, err
	}
	h, ok := m[root]
	return ok && h == hash, nil
}

// Trust records root with hash. A corrupt trust.json is an error and is left as it is, never overwritten.
func Trust(root, hash string) error {
	m, err := load()
	if err != nil {
		return err
	}
	m[root] = hash
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// CreateTemp makes the file 0600, which the rename keeps.
	f, err := os.CreateTemp(dir, ".trust-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name()) // no-op after a successful rename
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path())
}
