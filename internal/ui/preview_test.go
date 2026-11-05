package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/item"
)

func TestPreviewCIJobs(t *testing.T) {
	tests := []struct {
		name  string
		jobs  any // nil: field not set
		width int
		want  string // the ci_jobs line, "" when it must be absent
	}{
		{"strings", []string{"✗ lint", "⚠ e2e"}, 120, "ci_jobs     ✗ lint, ⚠ e2e"},
		{"decoded from the cache", []any{"✗ lint"}, 120, "ci_jobs     ✗ lint"},
		{"empty", []string{}, 120, ""},
		{"unset", nil, 120, ""},
		{"truncated at the pane width", []string{"✗ a-very-long-job-name"}, 24, "ci_jobs     ✗ a-very-l…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newNotesModel(t)
			m.width = tt.width
			f := map[string]any{"number": 7, "ci": "failed", "open_threads": 2}
			if tt.jobs != nil {
				f["ci_jobs"] = tt.jobs
			}
			m.data["p"] = []item.Item{{ID: "a", Fields: f}}
			m.rebuild()
			lines := strings.Split(ansi.Strip(m.preview()), "\n")
			idx := func(prefix string) int {
				for i, l := range lines {
					if strings.HasPrefix(strings.TrimSpace(l), prefix) {
						return i
					}
				}
				return -1
			}
			if tt.want == "" {
				if i := idx("ci_jobs"); i >= 0 {
					t.Fatalf("preview has a ci_jobs line:\n%s", strings.Join(lines, "\n"))
				}
				return
			}
			i := idx("ci_jobs")
			if i < 0 || strings.TrimSpace(lines[i]) != tt.want {
				t.Fatalf("ci_jobs line = %q, want %q:\n%s", lines[max(i, 0)], tt.want, strings.Join(lines, "\n"))
			}
			if ci := idx("ci "); ci < 0 || i != ci+1 {
				t.Errorf("ci_jobs at row %d, want right after ci (row %d)", i, ci)
			}
		})
	}
}
