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
	"github.com/chriha/lanes/internal/jsonfile"
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

// save writes the file atomically; a corrupt seen.json is moved to seen.json.bak first, so the first save
// after it is not lossy.
// ponytail: last writer wins across lanes instances (same as notes and snoozes).
func (s Store) save() error { return jsonfile.Write(path(), s, true) }
