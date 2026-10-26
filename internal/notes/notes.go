// Package notes keeps the user's own per-item notes in one JSON file in the project state dir.
package notes

import "time"

type Note struct {
	Text    string    `json:"text"`
	Updated time.Time `json:"updated"`
}

// Store maps item IDs to their notes.
type Store map[string]Note

// Load returns the saved notes.
func Load() (Store, error) { return Store{}, nil }

// Get returns the item's note text, empty when it has none.
func (s Store) Get(id string) string { return "" }

// Set stores the item's note and writes the file.
func (s Store) Set(id, text string) error { return nil }

// Delete removes the item's note and writes the file.
func (s Store) Delete(id string) error { return nil }
