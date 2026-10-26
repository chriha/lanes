package ui

import (
	"testing"
	"time"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/item"
)

func TestSortSurvivesRefresh(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	n := func(id string, v float64) item.Item { return item.Item{ID: id, Fields: map[string]any{"n": v}} }
	m := &Model{
		ov:        &config.Overrides{Filters: map[string]string{}},
		data:      map[string][]item.Item{"p": {n("a", 1), n("b", 2), n("c", 3)}},
		fetchedAt: map[string]time.Time{},
		loading:   map[string]bool{},
		marks:     map[string]bool{"a": true},
	}
	m.applyConfig(&config.Config{View: []config.View{{Name: "v", Provider: "p", Sort: "-n"}}})
	if got := ids(m.items()); got != "c, b, a" {
		t.Fatalf("initial order = %s, want c, b, a", got)
	}
	m.cursor = 1 // b

	m.Update(fetchedMsg{name: "p", items: []item.Item{n("a", 5), n("b", 4), n("c", 3)}})
	if got := ids(m.items()); got != "a, b, c" {
		t.Fatalf("order after refresh = %s, want a, b, c", got)
	}
	if got := m.selectedID(); got != "b" {
		t.Errorf("cursor on %s, want b", got)
	}
	if !m.marks["a"] || len(m.marks) != 1 {
		t.Errorf("marks = %v, want only a", m.marks)
	}
}
