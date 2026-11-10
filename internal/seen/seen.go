// Package seen keeps, per item, the fingerprint the user last saw, in one JSON file in the project state dir.
// An item whose current fingerprint differs from the stored one, or that has none, is unread.
package seen

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/chriha/lanes/internal/app"
)

// Store maps item IDs to the fingerprint the user last saw. Entries of items that left the list are kept.
type Store map[string]string

func path() string { return filepath.Join(app.StateDir(), "seen.json") }

// Load returns the saved store. first reports that there was no seen.json yet: the first run with the
// feature, when the caller records every current item as seen. A corrupt file yields an empty store and an
// error, and stays on disk until the next save moves it to seen.json.bak.
func Load() (s Store, first bool, err error) {
	s = Store{}
	b, err := os.ReadFile(path())
	if errors.Is(err, fs.ErrNotExist) {
		return s, true, nil
	}
	if err == nil {
		err = json.Unmarshal(b, &s)
	}
	if err != nil {
		return Store{}, false, fmt.Errorf("%s: %w", path(), err)
	}
	if s == nil { // the file held `null`
		s = Store{}
	}
	return s, false, nil
}

// Unread reports whether the item changed since the user last saw it, or was never seen.
func (s Store) Unread(id, fingerprint string) bool {
	fp, ok := s[id]
	return !ok || fp != fingerprint
}

// MarkRead records the items (ID → current fingerprint) as seen. It writes the file only when an entry changed.
func (s Store) MarkRead(fingerprints map[string]string) error {
	changed := false
	for id, fp := range fingerprints {
		if old, ok := s[id]; !ok || old != fp {
			s[id] = fp
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.save()
}

// MarkUnread forgets that the user saw the items. It writes the file only when an entry was removed.
func (s Store) MarkUnread(ids ...string) error {
	n := len(s)
	for _, id := range ids {
		delete(s, id)
	}
	if len(s) == n {
		return nil
	}
	return s.save()
}

// save writes through a temp file and a rename, so a crash never leaves a half-written seen.json.
// A corrupt seen.json is moved to seen.json.bak first, so the first save after it is not lossy.
// ponytail: copy of snooze.save; last writer wins across lanes instances (same as notes and snoozes).
func (s Store) save() error {
	dir := filepath.Dir(path())
	if err := os.MkdirAll(dir, app.DirMode); err != nil {
		return err
	}
	// Only a file that reads but doesn't decode is corrupt; a read error must not move a healthy file aside.
	if b, err := os.ReadFile(path()); err == nil && json.Unmarshal(b, &Store{}) != nil {
		if err := os.Rename(path(), path()+".bak"); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".seen-*.json")
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
