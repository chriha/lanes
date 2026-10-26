package ui

import (
	"errors"
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/item"
)

func mkItems(ids ...string) []item.Item {
	out := make([]item.Item, len(ids))
	for i, id := range ids {
		out[i] = item.Item{ID: id, Title: id, Fields: map[string]any{}}
	}
	return out
}

func itemIDs(its []item.Item) []string {
	out := make([]string, len(its))
	for i, it := range its {
		out[i] = it.ID
	}
	return out
}

func nav(m *Model, keys ...string) {
	for _, k := range keys {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		switch k {
		case "ctrl+d":
			msg = tea.KeyMsg{Type: tea.KeyCtrlD}
		case "ctrl+u":
			msg = tea.KeyMsg{Type: tea.KeyCtrlU}
		case " ":
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
		}
		m.key(msg)
	}
}

func TestInherit(t *testing.T) {
	withField := func(id, f string, v any, pending ...string) item.Item {
		return item.Item{ID: id, Fields: map[string]any{f: v}, Pending: pending}
	}
	tests := []struct {
		name       string
		old, fresh []item.Item
		wantField  any // fresh[0].Fields["ci"]
	}{
		{"pending field is carried over", []item.Item{withField("a", "ci", "pass")}, []item.Item{withField("a", "ci", nil, "ci")}, "pass"},
		{"non-pending field is not overwritten", []item.Item{withField("a", "ci", "pass")}, []item.Item{withField("a", "ci", "fail")}, "fail"},
		{"pending but unknown before stays unset", []item.Item{withField("a", "other", 1)}, []item.Item{{ID: "a", Fields: map[string]any{}, Pending: []string{"ci"}}}, nil},
		{"new item is untouched", []item.Item{withField("b", "ci", "pass")}, []item.Item{withField("a", "ci", "x", "ci")}, "x"},
		{"empty snapshot", nil, []item.Item{withField("a", "ci", "x", "ci")}, "x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inherit(tt.old, tt.fresh)
			if v := got[0].Fields["ci"]; v != tt.wantField {
				t.Errorf("ci = %v, want %v", v, tt.wantField)
			}
		})
	}
}

func TestFetchedMsg(t *testing.T) {
	boom := errors.New("boom")
	t.Run("full result stores items, stamps time, clears loading", func(t *testing.T) {
		m := newBackoffModel(t)
		m.loading["p"] = true
		m.Update(fetchedMsg{name: "p", items: mkItems("a", "b")})
		if got := itemIDs(m.items()); !reflect.DeepEqual(got, []string{"a", "b"}) {
			t.Errorf("items = %v", got)
		}
		if m.loading["p"] || m.fetchedAt["p"].IsZero() {
			t.Errorf("loading=%v fetchedAt=%v", m.loading["p"], m.fetchedAt["p"])
		}
	})
	t.Run("partial result shows items but keeps loading and no timestamp", func(t *testing.T) {
		m := newBackoffModel(t)
		m.loading["p"] = true
		m.Update(fetchedMsg{name: "p", items: mkItems("a"), partial: true})
		if len(m.items()) != 1 {
			t.Errorf("items = %v", itemIDs(m.items()))
		}
		if !m.loading["p"] || !m.fetchedAt["p"].IsZero() {
			t.Errorf("loading=%v fetchedAt=%v", m.loading["p"], m.fetchedAt["p"])
		}
	})
	t.Run("partial then full inherits pending fields", func(t *testing.T) {
		m := newBackoffModel(t)
		m.Update(fetchedMsg{name: "p", items: []item.Item{{ID: "a", Fields: map[string]any{"ci": "pass"}}}})
		m.Update(fetchedMsg{name: "p", partial: true, items: []item.Item{{ID: "a", Fields: map[string]any{}, Pending: []string{"ci"}}}})
		if v := m.items()[0].Fields["ci"]; v != "pass" {
			t.Errorf("ci = %v, want pass", v)
		}
	})
	t.Run("error sets provErr and keeps items", func(t *testing.T) {
		m := newBackoffModel(t)
		m.Update(fetchedMsg{name: "p", items: mkItems("a")})
		m.loading["p"] = true
		m.Update(fetchedMsg{name: "p", err: boom})
		if m.provErr["p"] != boom || m.fails["p"] != 1 || m.loading["p"] {
			t.Errorf("provErr=%v fails=%d loading=%v", m.provErr["p"], m.fails["p"], m.loading["p"])
		}
		if len(m.items()) != 1 {
			t.Errorf("items lost on error: %v", itemIDs(m.items()))
		}
	})
	t.Run("partial error is ignored while loading continues", func(t *testing.T) {
		m := newBackoffModel(t)
		m.loading["p"] = true
		m.Update(fetchedMsg{name: "p", err: boom, partial: true})
		if m.provErr["p"] != nil || m.fails["p"] != 0 || !m.loading["p"] {
			t.Errorf("provErr=%v fails=%d loading=%v", m.provErr["p"], m.fails["p"], m.loading["p"])
		}
	})
	t.Run("success clears provErr", func(t *testing.T) {
		m := newBackoffModel(t)
		m.Update(fetchedMsg{name: "p", err: boom})
		m.Update(fetchedMsg{name: "p", items: mkItems("a")})
		if m.provErr["p"] != nil || m.fails["p"] != 0 {
			t.Errorf("provErr=%v fails=%d", m.provErr["p"], m.fails["p"])
		}
	})
	t.Run("partial success clears provErr but not the backoff", func(t *testing.T) {
		m := newBackoffModel(t)
		m.Update(fetchedMsg{name: "p", err: boom})
		m.Update(fetchedMsg{name: "p", items: mkItems("a"), partial: true})
		if m.provErr["p"] != nil || m.fails["p"] != 1 {
			t.Errorf("provErr=%v fails=%d", m.provErr["p"], m.fails["p"])
		}
	})
}

func TestRefreshKeepsCursorAndMarks(t *testing.T) {
	tests := []struct {
		name       string
		refreshed  []string
		wantCursor string
		wantMarked []string
	}{
		{"item added above", []string{"x", "a", "b", "c", "d"}, "c", []string{"b", "d"}},
		{"item removed above", []string{"b", "c", "d"}, "c", []string{"b", "d"}},
		{"reordered", []string{"d", "c", "b", "a"}, "c", []string{"d", "b"}},
		{"cursor item removed", []string{"a", "b", "d"}, "d", []string{"b", "d"}}, // index 2 survives
		{"cursor item removed at end", []string{"a", "b"}, "b", []string{"b"}},
		{"marked item removed", []string{"a", "c", "d"}, "c", []string{"d"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBackoffModel(t)
			m.Update(fetchedMsg{name: "p", items: mkItems("a", "b", "c", "d")})
			nav(m, "j", " ") // mark b, cursor on c
			m.marks["d"] = true
			if m.selectedID() != "c" {
				t.Fatalf("setup: selected %q", m.selectedID())
			}
			m.Update(fetchedMsg{name: "p", items: mkItems(tt.refreshed...)})
			if got := m.selectedID(); got != tt.wantCursor {
				t.Errorf("cursor on %q, want %q", got, tt.wantCursor)
			}
			got := map[string]bool{}
			for id := range m.marks {
				got[id] = true
			}
			// marks are keyed by ID and are not pruned when an item disappears
			want := map[string]bool{"b": true, "d": true}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("marks = %v, want %v", got, want)
			}
			var visible []string
			for _, it := range m.items() {
				if m.marks[it.ID] {
					visible = append(visible, it.ID)
				}
			}
			if !reflect.DeepEqual(visible, tt.wantMarked) && !(len(visible) == 0 && len(tt.wantMarked) == 0) {
				t.Errorf("visible marked = %v, want %v", visible, tt.wantMarked)
			}
		})
	}
}

func TestNavigationClamps(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want int
	}{
		{"j moves down", []string{"j"}, 1},
		{"j clamps at the end", []string{"j", "j", "j", "j", "j", "j", "j"}, 4},
		{"k clamps at the start", []string{"k"}, 0},
		{"k moves up", []string{"j", "j", "k"}, 1},
		{"G goes to the end", []string{"G"}, 4},
		{"gg goes to the start", []string{"G", "g", "g"}, 0},
		{"single g does nothing", []string{"G", "g"}, 4},
		{"g interrupted by j starts over", []string{"G", "g", "k", "g"}, 3},
		{"ctrl+d clamps at the end", []string{"ctrl+d"}, 4},
		{"ctrl+u clamps at the start", []string{"j", "ctrl+u"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBackoffModel(t)
			m.Update(fetchedMsg{name: "p", items: mkItems("a", "b", "c", "d", "e")})
			nav(m, tt.keys...)
			if m.cursor != tt.want {
				t.Errorf("cursor = %d, want %d", m.cursor, tt.want)
			}
		})
	}
	t.Run("half page moves by listHeight/2", func(t *testing.T) {
		m := newBackoffModel(t)
		m.height = 8 // list height 4, half page 2
		m.Update(fetchedMsg{name: "p", items: mkItems("a", "b", "c", "d", "e", "f")})
		nav(m, "ctrl+d")
		if m.cursor != 2 {
			t.Errorf("after ctrl+d cursor = %d, want 2", m.cursor)
		}
		nav(m, "ctrl+u")
		if m.cursor != 0 {
			t.Errorf("after ctrl+u cursor = %d, want 0", m.cursor)
		}
	})
	t.Run("empty list", func(t *testing.T) {
		m := newBackoffModel(t)
		nav(m, "j", "G", "ctrl+d", "k")
		if m.cursor != 0 {
			t.Errorf("cursor = %d, want 0", m.cursor)
		}
	})
}
