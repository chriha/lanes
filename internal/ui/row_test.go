package ui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/item"
)

func TestRowPrefix(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(0) // termenv.TrueColor: render the colors the test compares
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	bg := regexp.MustCompile(`48;[0-9;]+`).FindString(lipgloss.NewStyle().Background(lipgloss.Color("#112233")).Render("x"))
	if bg == "" {
		t.Fatal("test needs a color profile: styles rendered plain")
	}
	tests := []struct {
		name     string
		selected bool
		marked   bool
		want     string // plain prefix before the ID
	}{
		{"unmarked", false, false, "  "},
		{"marked", false, true, "▌ "},
		{"selected", true, false, "  "},
		{"selected and marked", true, true, "▌ "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBackoffModel(t)
			useSelection(t, "#112233")
			its := mkItems("!1948")
			if tt.marked {
				m.marks[its[0].ID] = true
			}
			line := m.row(its[0], tt.selected, idColWidth(its))
			plain := ansi.Strip(line)
			if !strings.HasPrefix(plain, tt.want+"!1948") {
				t.Errorf("row = %q, want prefix %q before the ID", plain, tt.want)
			}
			if strings.ContainsAny(plain, "▸●") {
				t.Errorf("row = %q, has a retired glyph", plain)
			}
			if got := ansi.StringWidth(line); tt.selected && got != m.width {
				t.Errorf("selected row width %d, want %d", got, m.width)
			}
			if tt.selected {
				for _, chunk := range strings.Split(line, "\x1b[0m") {
					if ansi.Strip(chunk) != "" && !strings.Contains(chunk, bg) {
						t.Errorf("chunk %q of %q has no selection background", chunk, line)
					}
				}
			}
		})
	}
}

func TestRowFillsWidth(t *testing.T) {
	for _, selected := range []bool{false, true} {
		m := newBackoffModel(t)
		its := mkItems("!1948")
		its[0].Title = "a title"
		line := m.row(its[0], selected, idColWidth(its))
		want := m.width - 1 // the unselected row keeps a one-column right margin; the highlight fills it
		if selected {
			want = m.width
		}
		if got := ansi.StringWidth(line); got != want {
			t.Errorf("selected=%v: row width %d, want %d", selected, got, want)
		}
	}
}

// markerModel is a model whose config carries the given markers, applied like a reload would.
func markerModel(t *testing.T, width int, markers ...config.Marker) *Model {
	t.Helper()
	m := newBackoffModel(t)
	m.width = width
	m.applyConfig(markerConfig(markers))
	return m
}

func markerConfig(markers []config.Marker) *config.Config {
	return &config.Config{
		Provider: map[string]config.Provider{"p": {Type: "command", Command: "true"}},
		View:     []config.View{{Name: "v", Provider: "p", Filter: "true"}},
		Marker:   markers,
	}
}

func TestRowMarkers(t *testing.T) {
	ai := config.Marker{Text: "🤖", Labels: []string{"review::ai", "bot"}}
	human := config.Marker{Text: "🧑", Labels: []string{"Review::Human"}}
	tests := []struct {
		name    string
		markers []config.Marker
		fields  map[string]any
		author  string
		want    string // substring of the stripped row
		not     string // must be absent
	}{
		{"match after the author", []config.Marker{ai}, map[string]any{"labels": []string{"review::ai"}}, "bob", "fix it  bob 🤖", ""},
		{"no match", []config.Marker{ai}, map[string]any{"labels": []string{"other"}}, "bob", "fix it  bob", "🤖"},
		{"case-insensitive both ways", []config.Marker{human}, map[string]any{"labels": []string{"REVIEW::human"}}, "bob", "bob 🧑", ""},
		{"one marker for several labels", []config.Marker{ai}, map[string]any{"labels": []string{"x", "bot"}}, "bob", "bob 🤖", ""},
		{"config order, not label order", []config.Marker{ai, human}, map[string]any{"labels": []string{"review::human", "review::ai"}}, "bob", "bob 🤖 🧑", ""},
		{"labels decoded from JSON", []config.Marker{ai}, map[string]any{"labels": []any{"review::ai", 3}}, "bob", "bob 🤖", ""},
		{"labels missing", []config.Marker{ai}, map[string]any{}, "bob", "fix it  bob", "🤖"},
		{"labels not a list", []config.Marker{ai}, map[string]any{"labels": "review::ai"}, "bob", "fix it  bob", "🤖"},
		{"no name", []config.Marker{ai, human}, map[string]any{"labels": []string{"bot", "review::human"}}, "", "fix it  🤖 🧑", ""},
		{"before the priority strip", []config.Marker{ai}, map[string]any{"kind": "ticket", "priority": 1, "assignees": []string{"bob"}, "labels": []string{"bot"}}, "", "bob 🤖 ▰▰▰", ""},
		{"no name, strip follows", []config.Marker{ai}, map[string]any{"kind": "ticket", "priority": 1, "labels": []string{"bot"}}, "", "fix it  🤖 ▰▰▰", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := markerModel(t, 80, tt.markers...)
			it := item.Item{ID: "#1", Title: "fix it", Author: tt.author, Fields: tt.fields}
			line := ansi.Strip(m.row(it, false, 7))
			if !strings.Contains(line, tt.want) || (tt.not != "" && strings.Contains(line, tt.not)) {
				t.Errorf("row = %q, want it to contain %q and not %q", line, tt.want, tt.not)
			}
			if w := ansi.StringWidth(line); w != 79 {
				t.Errorf("row width = %d, want 79", w)
			}
		})
	}
}

func TestRowMarkersTruncateTitleFirst(t *testing.T) {
	ai := config.Marker{Text: "AI", Labels: []string{"bot"}}
	for _, width := range []int{40, 30, 22} {
		m := markerModel(t, width, ai, config.Marker{Text: "🧑", Labels: []string{"bot"}})
		it := item.Item{ID: "#1", Title: strings.Repeat("long title ", 8), Author: "bob", Fields: map[string]any{"labels": []string{"bot"}}}
		line := ansi.Strip(m.row(it, false, 7))
		if !strings.Contains(line, "bob AI 🧑") {
			t.Errorf("width %d: row = %q, want the markers intact", width, line)
		}
		if !strings.Contains(line, "…") {
			t.Errorf("width %d: row = %q, want the title truncated", width, line)
		}
	}
}

func TestRowMarkerColor(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(0) // termenv.TrueColor
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	bg := regexp.MustCompile(`48;[0-9;]+`).FindString(lipgloss.NewStyle().Background(lipgloss.Color("#112233")).Render("x"))
	fg := regexp.MustCompile(`38;[0-9;]+`).FindString(lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")).Render("x"))
	if bg == "" || fg == "" {
		t.Fatal("test needs a color profile: styles rendered plain")
	}
	m := markerModel(t, 80, config.Marker{Text: "AI", Labels: []string{"bot"}, Color: "#ff0000"}, config.Marker{Text: "plain", Labels: []string{"bot"}})
	useSelection(t, "#112233") // after the config apply, which resets the theme
	it := item.Item{ID: "#1", Title: "fix it", Author: "bob", Fields: map[string]any{"labels": []string{"bot"}}}
	for _, selected := range []bool{false, true} {
		line := m.row(it, selected, 7)
		chunk := ""
		for _, c := range strings.Split(line, "\x1b[0m") {
			if strings.Contains(ansi.Strip(c), "AI") {
				chunk = c
			}
		}
		if !strings.Contains(chunk, fg) {
			t.Errorf("selected=%v: marker chunk %q has no foreground %q", selected, chunk, fg)
		}
		if selected {
			for _, c := range strings.Split(line, "\x1b[0m") {
				if ansi.Strip(c) != "" && !strings.Contains(c, bg) {
					t.Errorf("chunk %q has no selection background", c)
				}
			}
		}
	}
}

func TestRowMarkersFollowConfigReload(t *testing.T) {
	m := markerModel(t, 80, config.Marker{Text: "🤖", Labels: []string{"bot"}})
	it := item.Item{ID: "#1", Title: "fix it", Author: "bob", Fields: map[string]any{"labels": []string{"bot"}}}
	if line := ansi.Strip(m.row(it, false, 7)); !strings.Contains(line, "🤖") {
		t.Fatalf("row = %q, want the marker", line)
	}
	m.applyConfig(markerConfig([]config.Marker{{Text: "AI", Labels: []string{"bot"}}}))
	if line := ansi.Strip(m.row(it, false, 7)); !strings.Contains(line, "AI") || strings.Contains(line, "🤖") {
		t.Errorf("row after reload = %q, want AI instead of the old marker", line)
	}
	m.applyConfig(markerConfig(nil))
	if line := ansi.Strip(m.row(it, false, 7)); strings.Contains(line, "AI") {
		t.Errorf("row after removal = %q, want no marker", line)
	}
}
