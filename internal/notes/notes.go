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

// save writes through a temp file and a rename, so a crash never leaves a half-written notes.json.
func (s Store) save() error {
	dir := filepath.Dir(path())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".notes-*.json")
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
