package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/notes"
)

// newNotesModel has three views on one provider: all items, only "a", and only noted items.
func newNotesModel(t *testing.T) *Model {
	t.Helper()
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "HOME"} {
		t.Setenv(k, t.TempDir())
	}
	return startNotesModel()
}

// startNotesModel builds the model on the current environment, as a restart would.
func startNotesModel() *Model {
	m := New(&config.Config{
		Provider: map[string]config.Provider{"p": {Type: "command", Command: "true"}},
		View: []config.View{
			{Name: "all", Provider: "p", Filter: "true"},
			{Name: "one", Provider: "p", Filter: `id == "a"`},
			{Name: "noted", Provider: "p", Filter: `note != ""`},
		},
	})
	m.width, m.height = 120, 40
	m.data["p"] = mkItems("a", "b")
	m.rebuild()
	return m
}

func command(m *Model, line string) {
	m.runCommand(line)
}

// rowOf renders the row of item id in the current view, stripped of styling.
func rowOf(t *testing.T, m *Model, id string) string {
	t.Helper()
	for _, it := range m.items() {
		if it.ID == id {
			return ansi.Strip(m.row(it, false))
		}
	}
	t.Fatalf("item %s not in view %s", id, m.views[m.cur].cfg.Name)
	return ""
}

func TestNoteCommand(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  string // stored note of a
	}{
		{"sets", []string{"note waiting on backend"}, "waiting on backend"},
		{"replaces", []string{"note one", "note two"}, "two"},
		{"clears", []string{"note one", "note"}, ""},
		{"clearing without a note is fine", []string{"note"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newNotesModel(t)
			for _, l := range tt.lines {
				command(m, l)
			}
			s, err := notes.Load()
			if err != nil {
				t.Fatal(err)
			}
			if got := s.Get("a"); got != tt.want {
				t.Errorf("stored note = %q, want %q", got, tt.want)
			}
			if got := s.Get("b"); got != "" {
				t.Errorf("b got a note: %q", got)
			}
		})
	}
}

func TestNoteShowsInEveryView(t *testing.T) {
	m := newNotesModel(t)
	if got := itemIDs(m.views[2].items); len(got) != 0 {
		t.Fatalf("noted view lists %v before any note", got)
	}
	command(m, "note waiting on backend")
	for i := range m.views {
		m.switchView(i)
		if !strings.Contains(rowOf(t, m, "a"), "✎") {
			t.Errorf("view %s: row of a lacks ✎: %q", m.views[i].cfg.Name, rowOf(t, m, "a"))
		}
	}
	m.switchView(0)
	if strings.Contains(rowOf(t, m, "b"), "✎") {
		t.Errorf("row of b has ✎ without a note")
	}
	if got := itemIDs(m.views[2].items); len(got) != 1 || got[0] != "a" {
		t.Errorf(`filter note != "" lists %v, want [a]`, got)
	}
}

func TestNotePreviewShowsFullText(t *testing.T) {
	m := newNotesModel(t)
	m.Update(noteEdited{id: "a", text: "waiting on backend\nre-review after Fri\n"})
	got := ansi.Strip(m.preview())
	for _, want := range []string{"waiting on backend", "re-review after Fri"} {
		if !strings.Contains(got, want) {
			t.Errorf("preview lacks %q:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "waiting on backend"); n != 1 {
		t.Errorf("note shown %d times in the preview:\n%s", n, got)
	}
}

func TestNoteInActionTemplates(t *testing.T) {
	m := newNotesModel(t)
	command(m, "note it's done")
	got, err := render("echo {{.note | q}}", m.targets()[0].Env())
	if err != nil {
		t.Fatal(err)
	}
	if want := `echo 'it'\''s done'`; got != want {
		t.Errorf("render = %q, want %q", got, want)
	}
}

func TestProviderNoteFieldWins(t *testing.T) {
	m := newNotesModel(t)
	m.data["p"] = []item.Item{{ID: "a", Fields: map[string]any{"note": "from provider"}}}
	m.rebuild()
	command(m, "note mine")
	if got := m.items()[0].Env()["note"]; got != "from provider" {
		t.Errorf("note = %v, want the provider's field", got)
	}
	if !strings.Contains(rowOf(t, m, "a"), "✎") {
		t.Errorf("row lacks ✎ for the user's note")
	}
	preview := ansi.Strip(m.preview())
	for _, want := range []string{"my note     mine", "note        from provider"} {
		if !strings.Contains(preview, want) {
			t.Errorf("preview lacks %q:\n%s", want, preview)
		}
	}
}

func TestNotesSurviveRefreshAndRestart(t *testing.T) {
	m := newNotesModel(t)
	command(m, "note keep me")
	m.Update(fetchedMsg{name: "p", items: mkItems("b", "a")})
	if !strings.Contains(rowOf(t, m, "a"), "✎") {
		t.Errorf("note lost on refresh")
	}
	if _, ok := m.data["p"][1].Fields["note"]; ok {
		t.Errorf("note leaked into the provider data (and its cache)")
	}
	m = startNotesModel()
	if !strings.Contains(rowOf(t, m, "a"), "✎") {
		t.Errorf("note lost on restart")
	}
}

func TestCorruptNotesFile(t *testing.T) {
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "HOME"} {
		t.Setenv(k, t.TempDir())
	}
	path := filepath.Join(app.StateDir(), "notes.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := startNotesModel()
	if !strings.Contains(ansi.Strip(m.footer()), "notes.json") {
		t.Errorf("footer does not report the corrupt file: %q", ansi.Strip(m.footer()))
	}
	m.rebuild()
	m.Update(fetchedMsg{name: "p", items: mkItems("a")})
	if b, _ := os.ReadFile(path); string(b) != "{oops" {
		t.Errorf("corrupt notes.json changed without a save: %q", b)
	}
}

func TestNoteKeyOpensEditor(t *testing.T) {
	m := newNotesModel(t)
	if _, cmd := m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("N")}); cmd == nil {
		t.Error("N returned no command")
	}
	m.data["p"] = nil
	m.rebuild()
	if _, cmd := m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("N")}); cmd != nil {
		t.Error("N on an empty view returned a command")
	}
}

func TestNoteFileIsPrefilled(t *testing.T) {
	m := newNotesModel(t)
	command(m, "note old text")
	path, err := m.noteFile("a")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	if b, _ := os.ReadFile(path); string(b) != "old text" {
		t.Errorf("note file = %q, want the current note", b)
	}
}

func TestReadNote(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		exitErr  error
		wantText string
		wantErr  bool
	}{
		{"saved text", "line 1\nline 2\n", nil, "line 1\nline 2\n", false},
		{"empty file", "", nil, "", false},
		{"editor failed", "half", errors.New("exit status 1"), "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "note.txt")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			raw := readNote("a", path, tt.exitErr)
			msg, ok := raw.(noteEdited)
			if !ok {
				t.Fatalf("readNote returned %T, want noteEdited", raw)
			}
			if msg.id != "a" || msg.text != tt.wantText || (msg.err != nil) != tt.wantErr {
				t.Errorf("got %+v, want text %q err %v", msg, tt.wantText, tt.wantErr)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("temp file not removed")
			}
		})
	}
}

func TestNoteEditedMsg(t *testing.T) {
	tests := []struct {
		name string
		msg  noteEdited
		want string
	}{
		{"saves trimmed text", noteEdited{id: "a", text: "  new\nlines\n\n"}, "new\nlines"},
		{"whitespace deletes", noteEdited{id: "a", text: " \n\t\n"}, ""},
		{"editor error keeps the old note", noteEdited{id: "a", text: "x", err: errors.New("exit status 1")}, "old"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newNotesModel(t)
			command(m, "note old")
			m.Update(tt.msg)
			s, err := notes.Load()
			if err != nil {
				t.Fatal(err)
			}
			if got := s.Get("a"); got != tt.want {
				t.Errorf("stored = %q, want %q", got, tt.want)
			}
			if got := m.items()[0].Env()["note"]; got != tt.want {
				t.Errorf("shown without refresh = %q, want %q", got, tt.want)
			}
			if tt.msg.err != nil && !strings.Contains(m.notice, "exit status 1") {
				t.Errorf("notice %q does not report the editor error", m.notice)
			}
		})
	}
}
