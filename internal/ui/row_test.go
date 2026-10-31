package ui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestRowPrefix(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(0) // termenv.TrueColor: render the colors the test compares
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	bg := regexp.MustCompile(`48;[0-9;]+`).FindString(lipgloss.NewStyle().Background(cSelBg).Render("x"))
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
