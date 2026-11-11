package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/seen"
)

// newSeenModel starts the model on a seen.json holding stored (ID → fingerprint), as a restart would.
func newSeenModel(t *testing.T, stored map[string]string) *Model {
	t.Helper()
	newNotesModel(t) // the environment
	if err := (seen.Store{}).MarkRead(stored); err != nil {
		t.Fatal(err)
	}
	return startNotesModel()
}

func itemOf(t *testing.T, m *Model, id string) item.Item {
	t.Helper()
	for _, it := range m.items() {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("item %s not in view %s", id, m.views[m.cur].cfg.Name)
	return item.Item{}
}

func storedSeen(t *testing.T) seen.Store {
	t.Helper()
	s, _, err := seen.Load()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestUnread(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(0) // termenv.TrueColor: render the bold the test looks for
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	a := mkItems("a")[0]
	tests := []struct {
		name   string
		stored map[string]string
		fields map[string]any
		want   bool
	}{
		{"seen at its fingerprint", map[string]string{"a": a.Fingerprint()}, nil, false},
		{"changed since", map[string]string{"a": "old"}, nil, true},
		{"never seen", map[string]string{"x": "old"}, nil, true},
		{"provider field unread wins", map[string]string{"a": "old"}, map[string]any{"unread": "provider"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newSeenModel(t, tt.stored)
			m.width = 80
			it := mkItems("a")[0]
			if tt.fields != nil {
				it.Fields = tt.fields
			}
			m.data["p"] = []item.Item{it}
			m.rebuild()

			got := itemOf(t, m, "a")
			wantEnv := any(tt.want)
			if tt.fields != nil {
				wantEnv = tt.fields["unread"]
			}
			if env := got.Env()["unread"]; env != wantEnv {
				t.Errorf("unread in filters = %v, want %v", env, wantEnv)
			}
			if bold := strings.Contains(m.row(got, false, 1), "\x1b[1;"); bold != tt.want {
				t.Errorf("bold title = %v, want %v", bold, tt.want)
			}
		})
	}
}

func TestUnreadFirstRunRecordsEveryItemSeen(t *testing.T) {
	m := newNotesModel(t) // no seen.json
	quick := mkItems("a", "b")
	quick[0].Pending = []string{"ci"}
	m.Update(fetchedMsg{name: "p", items: quick, partial: true})
	if itemOf(t, m, "a").Env()["unread"] != false {
		t.Error("a partial result on the first run shows unread")
	}
	full := mkItems("a", "b")
	full[0].Fields["ci"] = "ok"
	m.Update(fetchedMsg{name: "p", items: full})
	for _, id := range []string{"a", "b"} {
		if itemOf(t, m, id).Env()["unread"] != false {
			t.Errorf("%s unread after the first load", id)
		}
		if _, ok := storedSeen(t)[id]; !ok {
			t.Errorf("%s not recorded in seen.json", id)
		}
	}

	changed := mkItems("a", "b")
	changed[0].Fields["ci"] = "failed"
	m.Update(fetchedMsg{name: "p", items: changed})
	if itemOf(t, m, "a").Env()["unread"] != true {
		t.Error("a changed after the first load but is not unread")
	}
	if itemOf(t, m, "b").Env()["unread"] != false {
		t.Error("b did not change but is unread")
	}
}

func TestUnreadPartialResultKeepsItemRead(t *testing.T) {
	full := mkItems("a")
	full[0].Fields["ci"] = "ok"
	m := newSeenModel(t, map[string]string{"a": full[0].Fingerprint()})
	m.Update(fetchedMsg{name: "p", items: full})

	quick := mkItems("a")
	quick[0].Pending = []string{"ci"} // the quick pass can't compute ci: inherited from the full result
	m.Update(fetchedMsg{name: "p", items: quick, partial: true})
	if itemOf(t, m, "a").Env()["unread"] != false {
		t.Error("a partial result flipped the item to unread")
	}
}

func TestDwellMarksRead(t *testing.T) {
	tests := []struct {
		name     string
		keys     []string // pressed after the tick for a was armed
		wantRead bool
	}{
		{"cursor stays", nil, true},
		{"cursor moves away", []string{"j"}, false},
		{"cursor moves away and back", []string{"j", "k"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newSeenModel(t, map[string]string{"a": "old", "b": "old"})
			m.data["p"] = mkItems("a", "b")
			m.rebuild()
			m.Update(tea.WindowSizeMsg{Width: 80, Height: 20}) // the cursor lands on a
			gen := m.dwellGen
			for _, k := range tt.keys {
				m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
			}
			m.Update(dwellMsg{id: "a", gen: gen})

			if read := itemOf(t, m, "a").Env()["unread"] == false; read != tt.wantRead {
				t.Errorf("a read = %v, want %v", read, tt.wantRead)
			}
			if saved := storedSeen(t)["a"] != "old"; saved != tt.wantRead {
				t.Errorf("seen.json updated for a = %v, want %v", saved, tt.wantRead)
			}
			if itemOf(t, m, "b").Env()["unread"] != true {
				t.Error("b was marked read without the cursor resting on it")
			}
		})
	}
}

func TestMTogglesRead(t *testing.T) {
	fp := map[string]string{}
	for _, it := range mkItems("a", "b") {
		fp[it.ID] = it.Fingerprint()
	}
	read := func(ids ...string) map[string]string { // stored: ids read, the rest of a, b unread
		s := map[string]string{"a": "old", "b": "old"}
		for _, id := range ids {
			s[id] = fp[id]
		}
		return s
	}
	tests := []struct {
		name       string
		stored     map[string]string
		marks      []string
		wantUnread map[string]bool
	}{
		{"read cursor item turns unread", read("a", "b"), nil, map[string]bool{"a": true, "b": false}},
		{"unread cursor item turns read", read(), nil, map[string]bool{"a": false, "b": true}},
		{"marked, not all unread: all unread", read("a"), []string{"a", "b"}, map[string]bool{"a": true, "b": true}},
		{"marked, all unread: all read", read(), []string{"a", "b"}, map[string]bool{"a": false, "b": false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newSeenModel(t, tt.stored)
			m.data["p"] = mkItems("a", "b")
			m.rebuild()
			m.Update(tea.WindowSizeMsg{Width: 80, Height: 20}) // the cursor lands on a
			for _, id := range tt.marks {
				m.marks[id] = true
			}
			if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")}); cmd != nil {
				t.Error("m armed a command (a dwell tick would undo the choice)")
			}

			for id, want := range tt.wantUnread {
				if got := itemOf(t, m, id).Env()["unread"]; got != want {
					t.Errorf("%s unread = %v, want %v", id, got, want)
				}
				if saved := storedSeen(t).Unread(id, fp[id]); saved != want {
					t.Errorf("seen.json: %s unread = %v, want %v", id, saved, want)
				}
			}
			if len(m.marks) != 0 {
				t.Errorf("marks not cleared: %v", m.marks)
			}
		})
	}
}

func TestMIsBuiltIn(t *testing.T) {
	if !strings.Contains(builtinKeys, "m") {
		t.Error("m is not in builtinKeys")
	}
}
