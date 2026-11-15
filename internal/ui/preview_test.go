package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/filter"
	"github.com/chriha/lanes/internal/history"
	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/session"
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

func TestLastRun(t *testing.T) {
	tok := &history.Tokens{Input: 100_000, CacheWrite: 1_000_000, CacheRead: 9_000_000, Output: 1_000_000}
	tests := []struct {
		name string
		e    history.Event
		want string
	}{
		{"claude session finished", history.Event{Action: "work", Event: "finished", Dur: 14 * 60, Tokens: tok}, "work · 14m · 2.1M tok · ✓"},
		{"run failed", history.Event{Action: "fix", Event: "failed", Exit: 1, Dur: 42}, "fix · 42s · ✗ 1"},
		{"failed without an exit code", history.Event{Action: "fix", Event: "failed"}, "fix · ✗"},
		{"killed", history.Event{Action: "work", Event: "killed", Dur: 3900}, "work · 1h05m · killed"},
		{"interrupted", history.Event{Action: "work", Event: "interrupted", Dur: 5}, "work · 5s · interrupted"},
		{"started has no duration", history.Event{Action: "work", Event: "started"}, "work · started"},
		{"old line without dur or tokens", history.Event{Action: "work", Event: "finished"}, "work · ✓"},
		{"zero tokens left out", history.Event{Action: "work", Event: "finished", Dur: 60, Tokens: &history.Tokens{CacheRead: 500}}, "work · 1m · ✓"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lastRun(tt.e); got != tt.want {
				t.Errorf("lastRun() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunDur(t *testing.T) {
	tests := []struct {
		sec  int64
		want string
	}{
		{0, "0s"}, {42, "42s"}, {59, "59s"}, {60, "1m"}, {14*60 + 59, "14m"}, {3599, "59m"},
		{3600, "1h00m"}, {3900, "1h05m"}, {26 * 3600, "26h00m"},
	}
	for _, tt := range tests {
		if got := runDur(tt.sec); got != tt.want {
			t.Errorf("runDur(%d) = %q, want %q", tt.sec, got, tt.want)
		}
	}
}

func TestTokenCount(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"}, {850, "850"}, {999, "999"}, {1000, "1k"}, {1_250, "1.2k"}, {12_345, "12k"}, {999_999, "999k"},
		{1_000_000, "1M"}, {2_150_000, "2.1M"}, {12_900_000, "12M"},
	}
	for _, tt := range tests {
		if got := tokenCount(tt.n); got != tt.want {
			t.Errorf("tokenCount(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestPreviewLastRunLine(t *testing.T) {
	m := newNotesModel(t)
	m.width = 120
	m.data["p"] = []item.Item{{ID: "a", Fields: map[string]any{"number": 7}}}
	m.rebuild()
	history.Append(history.Event{Item: session.Key("a"), Action: "work", Event: "started"})
	history.Append(history.Event{Item: session.Key("a"), Action: "work", Event: "finished", Dur: 840, Tokens: &history.Tokens{Input: 12_000, Output: 500}})
	// a queued on_enter start is no run: the last run stays the finished one
	history.Append(history.Event{Item: session.Key("a"), Action: "review", Event: "auto"})
	for _, l := range strings.Split(ansi.Strip(m.preview()), "\n") {
		if l = strings.TrimSpace(l); strings.HasPrefix(l, "last run") {
			if want := "last run    work · 14m · 12k tok · ✓"; l != want {
				t.Errorf("last run line = %q, want %q", l, want)
			}
			return
		}
	}
	t.Fatal("preview has no last run line")
}

func TestPreviewStackFields(t *testing.T) {
	stack := []item.Item{
		{ID: "a", Fields: map[string]any{"number": 1, "branch": "feat/a", "target": "main"}},
		{ID: "b", Fields: map[string]any{"number": 2, "branch": "feat/b", "target": "feat/a"}},
		{ID: "c", Fields: map[string]any{"number": 3, "branch": "feat/c", "target": "main"}},
	}
	tests := []struct {
		name string
		id   string
		want []string // the lines after target, in order; nil when the item is in no stack
	}{
		{"stacked item", "b", []string{"base        a", "stack_pos   2", "stack_size  2"}},
		{"base of a stack", "a", []string{"stack_pos   1", "stack_size  2", "stacked_by  b"}},
		{"in no stack", "c", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newNotesModel(t)
			m.data["p"] = stack
			m.rebuild()
			m.cursor = -1
			for i, it := range m.items() {
				if it.ID == tt.id {
					m.cursor = i
				}
			}
			if m.cursor < 0 {
				t.Fatalf("item %q not listed", tt.id)
			}
			var lines []string
			for _, l := range strings.Split(ansi.Strip(m.preview()), "\n") {
				lines = append(lines, strings.TrimSpace(l))
			}
			at := -1
			for i, l := range lines {
				if strings.HasPrefix(l, "target ") {
					at = i
				}
			}
			if at < 0 {
				t.Fatalf("preview has no target line:\n%s", strings.Join(lines, "\n"))
			}
			for _, k := range []string{"base ", "stack_pos ", "stack_size ", "stacked_by "} {
				for _, l := range lines {
					if tt.want == nil && strings.HasPrefix(l, k) {
						t.Fatalf("preview of an unstacked item has %q:\n%s", l, strings.Join(lines, "\n"))
					}
				}
			}
			for i, w := range tt.want {
				if got := lines[at+1+i]; got != w {
					t.Errorf("line %d after target = %q, want %q:\n%s", i+1, got, w, strings.Join(lines, "\n"))
				}
			}
		})
	}
}

// sessionsPreview opens the sessions view with its preview on m, feeds it peek and returns the preview's lines.
func sessionsPreview(t *testing.T, m *Model, peek sessPeekMsg) []string {
	t.Helper()
	m.preview_ = true
	m.openSessions()
	m.Update(peek)
	out := m.preview()
	if n := strings.Count(out, "\n"); n != previewH {
		t.Fatalf("preview has %d lines, want %d", n, previewH)
	}
	return strings.Split(ansi.Strip(out), "\n")
}

func TestSessionPreview(t *testing.T) {
	created := time.Now().Add(-5 * time.Minute)
	tests := []struct {
		name   string
		info   session.Info
		peek   sessPeekMsg
		want   []string // lines the preview must have, without the leading pad and trailing spaces
		absent []string // line prefixes it must not have
	}{
		{"listed item",
			session.Info{Name: "fix~1", Action: "fix", ItemKey: "1", Created: created, Claude: "asking"},
			sessPeekMsg{name: "fix~1", dir: "/work/one", out: []string{"compiling", "done"}},
			[]string{
				"session     fix~1",
				"action      fix",
				"state       ? fix: asking  since " + created.Format("15:04") + " · 5m",
				"dir         /work/one",
				"item        1 first",
				"url         https://example.com/1",
				"output      compiling",
				"            done",
			}, nil},
		{"orphan session",
			session.Info{Name: "fix~9", Action: "fix", ItemKey: "9", Created: created, Dead: true, Exit: 2},
			sessPeekMsg{name: "fix~9", dir: "/work/nine", out: []string{"boom"}},
			[]string{
				"session     fix~9",
				"state       ✗ fix (2)  since " + created.Format("15:04") + " · 5m",
				"dir         /work/nine",
				"item        9  (not in any view)",
				"output      boom",
			}, []string{"url"}},
		{"peek of another session not shown",
			session.Info{Name: "fix~1", Action: "fix", ItemKey: "1", Created: created},
			sessPeekMsg{name: "review~1", dir: "/elsewhere", out: []string{"other"}},
			[]string{"session     fix~1"}, []string{"dir", "output"}},
		{"failed peek shows no directory or output",
			session.Info{Name: "fix~1", Action: "fix", ItemKey: "1", Created: created},
			sessPeekMsg{name: "fix~1", dir: "/work/one", out: []string{"x"}, err: errors.New("gone")},
			[]string{"session     fix~1", "item        1 first"}, []string{"dir", "output"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := sessModel(t, tt.info)
			m.data["p"] = []item.Item{{ID: "1", Title: "first", URL: "https://example.com/1"}}
			f, err := filter.Compile("true")
			if err != nil {
				t.Fatal(err)
			}
			m.views[0].cfg.Provider, m.views[0].filter = "p", f
			lines := sessionsPreview(t, m, tt.peek)
			for i := range lines {
				lines[i] = strings.TrimRight(strings.TrimPrefix(lines[i], " "), " ")
			}
			for _, w := range tt.want {
				if !slices.Contains(lines, w) {
					t.Errorf("preview lacks %q:\n%s", w, strings.Join(lines, "\n"))
				}
			}
			for _, a := range tt.absent {
				for _, l := range lines {
					if strings.HasPrefix(l, a) {
						t.Errorf("preview has %q:\n%s", l, strings.Join(lines, "\n"))
					}
				}
			}
		})
	}
}

func TestSessionPreviewKeepsLastOutputLines(t *testing.T) {
	m := sessModel(t, session.Info{Name: "fix~9", Action: "fix", ItemKey: "9", Created: time.Now()})
	var out []string
	for i := 1; i <= 20; i++ {
		out = append(out, fmt.Sprintf("line %d", i))
	}
	lines := sessionsPreview(t, m, sessPeekMsg{name: "fix~9", dir: "/w", out: out})
	if got := strings.TrimSpace(lines[previewH-1]); got != "line 20" {
		t.Errorf("last preview line = %q, want the newest output line:\n%s", got, strings.Join(lines, "\n"))
	}
}

func TestPollPeek(t *testing.T) {
	tests := []struct {
		name     string
		preview  bool
		onTab    bool
		sessions []session.Info
		peeking  bool
		want     bool
	}{
		{"preview on the sessions tab", true, true, names("fix~1"), false, true},
		{"preview off", false, true, names("fix~1"), false, false},
		{"other tab", true, false, names("fix~1"), false, false},
		{"no session", true, true, nil, false, false},
		{"one in flight", true, true, names("fix~1"), true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := sessModel(t, tt.sessions...)
			m.preview_ = tt.preview
			if tt.onTab {
				m.openSessions()
			}
			m.peeking = tt.peeking
			if got := m.pollPeek() != nil; got != tt.want {
				t.Errorf("pollPeek() != nil = %v, want %v", got, tt.want)
			}
		})
	}
}
