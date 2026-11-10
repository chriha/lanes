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

func TestPreviewKeepsCIJobsInCrowdedPane(t *testing.T) {
	crowded := map[string]any{
		"number": 7, "branch": "fix/x", "target": "main", "labels": []string{"bug"},
		"reviewers": []string{"bo"}, "assignees": []string{"al"}, "ci": "failed", "open_threads": 2,
	}
	tests := []struct {
		name     string
		note     string
		jobs     any    // nil: field not set
		want     string // the ci_jobs line, "" when it must be absent
		wantGone string // a field line trimmed to make room, "" for none
	}{
		{"fields fill the pane", "", []string{"✗ lint"}, "ci_jobs     ✗ lint", "assignees"},
		{"note and fields fill the pane", "one\ntwo\nthree", []any{"✗ lint"}, "ci_jobs     ✗ lint", "target"},
		{"empty jobs", "one\ntwo\nthree", []string{}, "", ""},
		{"unset jobs", "", nil, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newNotesModel(t)
			f := map[string]any{}
			for k, v := range crowded {
				f[k] = v
			}
			if tt.jobs != nil {
				f["ci_jobs"] = tt.jobs
			}
			m.data["p"] = []item.Item{{ID: "a", Title: "t", URL: "u", Author: "me", Fields: f}}
			m.rebuild()
			if tt.note != "" {
				if err := m.notes.Set("a", tt.note); err != nil {
					t.Fatal(err)
				}
			}
			out := m.preview()
			if n := strings.Count(out, "\n"); n != previewH {
				t.Fatalf("preview has %d lines, want %d", n, previewH)
			}
			lines := strings.Split(ansi.Strip(out), "\n")
			has := func(prefix string) string {
				for _, l := range lines {
					if strings.HasPrefix(strings.TrimSpace(l), prefix) {
						return strings.TrimSpace(l)
					}
				}
				return ""
			}
			if got := has("ci_jobs"); got != tt.want {
				t.Fatalf("ci_jobs line = %q, want %q:\n%s", got, tt.want, strings.Join(lines, "\n"))
			}
			if tt.wantGone != "" && has(tt.wantGone) != "" {
				t.Errorf("%q line kept, want it trimmed:\n%s", tt.wantGone, strings.Join(lines, "\n"))
			}
		})
	}
}
