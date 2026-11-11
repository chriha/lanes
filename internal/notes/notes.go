// Package notes keeps the user's own per-item notes in one JSON file in the project state dir.
package notes

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/jsonfile"
)

type Note struct {
	Text    string    `json:"text"`
	Updated time.Time `json:"updated"`
}

// Store maps item IDs to their notes. Notes of items that left the list are kept.
type Store map[string]Note

func path() string { return filepath.Join(app.StateDir(), "notes.json") }

// Load returns the saved notes. A missing file is an empty store; a corrupt one yields an empty
// store and an error, and stays on disk until the next Set or Delete replaces it.
func Load() (Store, error) {
	s := Store{}
	b, err := os.ReadFile(path())
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err == nil {
		err = json.Unmarshal(b, &s)
	}
	if err != nil {
		return Store{}, fmt.Errorf("%s: %w", path(), err)
	}
	return s, nil
}

// Get returns the item's note text, empty when it has none.
func (s Store) Get(id string) string { return s[id].Text }

// Set stores the item's note and writes the file; whitespace-only text deletes the note.
func (s Store) Set(id, text string) error {
	if strings.TrimSpace(text) == "" {
		delete(s, id)
	} else {
		s[id] = Note{Text: text, Updated: time.Now()}
	}
	return s.save()
}

// Delete removes the item's note and writes the file.
func (s Store) Delete(id string) error { return s.Set(id, "") }

// save writes the file atomically; a corrupt notes.json is moved to notes.json.bak first, so the first save
// after it is not lossy.
// ponytail: last writer wins across lanes instances; re-read under a file lock if that ever bites.
func (s Store) save() error { return jsonfile.Write(path(), s, true) }
