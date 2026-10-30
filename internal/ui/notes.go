package ui

import (
	"maps"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/item"
)

// noteEdited carries the text the editor left in an item's note file.
type noteEdited struct {
	id, text string
	err      error
}

// withNote returns the item with its note in a copy of its fields, so filters and templates see it.
// The provider's data is left alone (it is cached); a provider field called note wins.
func (m *Model) withNote(it item.Item) item.Item {
	text := m.notes.Get(it.ID)
	if text == "" {
		return it
	}
	if _, ok := it.Fields["note"]; ok {
		return it
	}
	f := make(map[string]any, len(it.Fields)+1)
	maps.Copy(f, it.Fields)
	f["note"] = text
	it.Fields = f
	return it
}

// editNote opens the item's note in the editor; the result comes back as noteEdited.
func (m *Model) editNote(id string) tea.Cmd {
	path, err := m.noteFile(id)
	if err != nil {
		m.say(lvErr, "note: "+err.Error())
		return nil
	}
	cmd := exec.Command("/bin/sh", "-c", editor()+" "+shq(path))
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return readNote(id, path, err) })
}

// noteFile writes the item's current note to a temp file for the editor.
func (m *Model) noteFile(id string) (string, error) {
	f, err := os.CreateTemp("", app.Name+"-note-*.txt")
	if err != nil {
		return "", err
	}
	_, err = f.WriteString(m.notes.Get(id))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// readNote turns the editor's result into a noteEdited message and removes the temp file.
// A failed editor (e.g. vim's :cq) carries its error and leaves the note as it was.
func readNote(id, path string, err error) tea.Msg {
	defer os.Remove(path)
	if err != nil {
		return noteEdited{id: id, err: err}
	}
	b, err := os.ReadFile(path)
	return noteEdited{id: id, text: string(b), err: err}
}

// setNote stores the item's note (whitespace-only deletes it) and shows it right away.
func (m *Model) setNote(id, text string) {
	text = strings.TrimSpace(text)
	if err := m.notes.Set(id, text); err != nil {
		m.say(lvErr, "saving note: "+err.Error())
		return
	}
	m.say(lvInfo, "note saved on "+id)
	if text == "" {
		m.say(lvInfo, "note removed from "+id)
	}
	m.rebuild()
}
