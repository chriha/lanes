package ui

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"
)

// noteEdited carries the text the editor left in an item's note file.
type noteEdited struct {
	id, text string
	err      error
}

// noteFile writes the item's current note to a temp file for the editor.
func (m *Model) noteFile(id string) (string, error) { return "", errors.New("not implemented") }

// readNote turns the editor's result into a noteEdited message and removes the temp file.
func readNote(id, path string, err error) tea.Msg { return nil }
