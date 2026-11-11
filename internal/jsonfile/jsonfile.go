// Package jsonfile writes the JSON files of lanes' small stores (notes, snoozes, seen, trust) atomically.
package jsonfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/chriha/lanes/internal/app"
)

// Write writes v as indented JSON to path through a temp file in the same dir and a rename, so a crash never
// leaves a half-written file. It creates the dir as app.DirMode; the file ends up as app.FileMode.
//
// With backupCorrupt, an existing file that reads but does not decode into T is moved to path+".bak" first, so the
// first write after it is not lossy. A read error never moves a healthy file aside.
func Write[T any](path string, v T, backupCorrupt bool) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, app.DirMode); err != nil {
		return err
	}
	if backupCorrupt {
		var probe T
		if b, err := os.ReadFile(path); err == nil && json.Unmarshal(b, &probe) != nil {
			if err := os.Rename(path, path+".bak"); err != nil {
				return err
			}
		}
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	f, err := os.CreateTemp(dir, "."+strings.TrimSuffix(base, ext)+"-*"+ext)
	if err != nil {
		return err
	}
	defer os.Remove(f.Name()) // no-op after a successful rename
	if err := f.Chmod(app.FileMode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
