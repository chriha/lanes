package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/item"
)

func TestDetailLines(t *testing.T) {
	long := strings.Repeat("x", 150)
	tests := []struct {
		name   string
		fields map[string]any
		note   string
		want   []string // lines that must be present, trimmed
		absent []string // line prefixes that must not be present
	}{
		{
			name:   "ci_jobs one per line",
			fields: map[string]any{"ci": "failed", "ci_jobs": []string{"✗ lint", "⚠ e2e"}},
			want:   []string{"ci          failed", "ci_jobs     ✗ lint", "⚠ e2e"},
		},
		{
			name:   "ci_jobs decoded from the cache",
			fields: map[string]any{"ci_jobs": []any{"✗ lint", "✗ unit"}},
			want:   []string{"ci_jobs     ✗ lint", "✗ unit"},
		},
		{
			name:   "empty ci_jobs",
			fields: map[string]any{"ci": "passed", "ci_jobs": []string{}},
			want:   []string{"ci          passed"},
			absent: []string{"ci_jobs"},
		},
		{
			name:   "every field, also the ones the preview leaves out",
			fields: map[string]any{"number": 7, "zeta": "z", "branch": long},
			note:   "first\nsecond",
			want:   []string{"t-title", "https://x/7", "author      me", "note        first", "second", "number      7", "zeta        z"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newNotesModel(t)
			m.width, m.height = 200, 40
			m.data["p"] = []item.Item{{ID: "a", Title: "t-title", URL: "https://x/7", Author: "me", Fields: tt.fields}}
			m.rebuild()
			if tt.note != "" {
				if err := m.notes.Set("a", tt.note); err != nil {
					t.Fatal(err)
				}
			}
			m.showDetail()
			var lines []string
			for _, l := range m.detailLines() {
				lines = append(lines, strings.TrimSpace(ansi.Strip(l)))
			}
			all := strings.Join(lines, "\n")
			for _, w := range tt.want {
				found := false
				for _, l := range lines {
					found = found || strings.HasSuffix(l, w) || l == w
				}
				if !found {
					t.Errorf("no line %q in:\n%s", w, all)
				}
			}
			for _, a := range tt.absent {
				for _, l := range lines {
					if strings.HasPrefix(l, a) {
						t.Errorf("line %q present, want none starting with %q", l, a)
					}
				}
			}
			if v, ok := tt.fields["branch"].(string); ok && !strings.Contains(strings.ReplaceAll(all, "\n", ""), v) {
				t.Errorf("branch value truncated:\n%s", all)
			}
		})
	}
}

func TestDetailKeys(t *testing.T) {
	jobs := make([]string, 10)
	for i := range jobs {
		jobs[i] = "✗ job" + string(rune('0'+i))
	}
	tests := []struct {
		name      string
		items     []item.Item
		keys      []string
		wantMode  mode
		wantFirst string // first screen line, trimmed; "" to skip
		wantLast  string // last content line on screen, trimmed; "" to skip
	}{
		{"d opens", nil, []string{"d"}, modeDetail, "a  t-title", ""},
		{"no items: d does nothing", []item.Item{}, []string{"d"}, modeNormal, "", ""},
		{"j scrolls down", nil, []string{"d", "j"}, modeDetail, "https://x/7", ""},
		{"k scrolls back up", nil, []string{"d", "j", "k"}, modeDetail, "a  t-title", ""},
		{"G to the bottom", nil, []string{"d", "G"}, modeDetail, "", "✗ job9"},
		{"g to the top", nil, []string{"d", "G", "g"}, modeDetail, "a  t-title", ""},
		{"ctrl-d pages down", nil, []string{"d", "ctrl+d"}, modeDetail, "author      me", ""},
		{"ctrl-u pages up", nil, []string{"d", "ctrl+d", "ctrl+u"}, modeDetail, "a  t-title", ""},
		{"q closes", nil, []string{"d", "q"}, modeNormal, "", ""},
		{"esc closes", nil, []string{"d", "esc"}, modeNormal, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newNotesModel(t)
			m.width, m.height = 80, 7 // 6 content lines and the footer
			its := tt.items
			if its == nil {
				its = []item.Item{{ID: "a", Title: "t-title", URL: "https://x/7", Author: "me", Fields: map[string]any{"ci_jobs": jobs}}}
			}
			m.data["p"] = its
			m.rebuild()
			for _, k := range tt.keys {
				if k == "esc" {
					m.key(tea.KeyMsg{Type: tea.KeyEsc})
					continue
				}
				nav(m, k)
			}
			if m.mode != tt.wantMode {
				t.Fatalf("mode = %d, want %d", m.mode, tt.wantMode)
			}
			if tt.wantMode != modeDetail {
				return
			}
			screen := strings.Split(ansi.Strip(m.View()), "\n")
			if n := len(screen); n != m.height {
				t.Fatalf("detail screen has %d lines, want %d:\n%s", n, m.height, strings.Join(screen, "\n"))
			}
			if tt.wantFirst != "" && strings.TrimSpace(screen[0]) != tt.wantFirst {
				t.Errorf("first line = %q, want %q:\n%s", screen[0], tt.wantFirst, strings.Join(screen, "\n"))
			}
			if tt.wantLast != "" && strings.TrimSpace(screen[len(screen)-2]) != tt.wantLast {
				t.Errorf("last line = %q, want %q:\n%s", screen[len(screen)-2], tt.wantLast, strings.Join(screen, "\n"))
			}
		})
	}
}
