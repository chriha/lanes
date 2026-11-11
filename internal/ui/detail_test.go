package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

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
			want:   []string{"ci_jobs  ✗ lint", "⚠ e2e"},
		},
		{
			name:   "ci_jobs decoded from the cache",
			fields: map[string]any{"ci_jobs": []any{"✗ lint", "✗ unit"}},
			want:   []string{"ci_jobs  ✗ lint", "✗ unit"},
		},
		{
			name:   "empty ci_jobs",
			fields: map[string]any{"ci": "passed", "ci_jobs": []string{}},
			want:   []string{"by me · ci passed"},
			absent: []string{"ci_jobs"},
		},
		{
			name:   "every field, also the ones the preview leaves out",
			fields: map[string]any{"number": 7, "zeta": "z", "branch": long},
			note:   "first\nsecond",
			want:   []string{"t-title", "https://x/7", "by me", "note    first", "second", "number  7", "zeta    z"},
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
		{"ctrl-d pages down", nil, []string{"d", "ctrl+d", "ctrl+d"}, modeDetail, "✗ job1", ""},
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

func TestDetailHeader(t *testing.T) {
	tests := []struct {
		name   string
		draft  bool
		fields map[string]any
		want   string // third line, trimmed
	}{
		{"author only", false, nil, "by me"},
		{"pipeline state", false, map[string]any{"ci": "failed"}, "by me · ci failed"},
		{"draft", true, map[string]any{"ci": "running"}, "by me · ci running · draft"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newNotesModel(t)
			m.width, m.height = 80, 40
			m.data["p"] = []item.Item{{ID: "a", Title: "t-title", URL: "https://x/7", Author: "me", Draft: tt.draft, Fields: tt.fields}}
			m.rebuild()
			m.showDetail()
			lines := m.detailLines()
			if got := strings.TrimSpace(ansi.Strip(lines[2])); got != tt.want {
				t.Errorf("header state = %q, want %q", got, tt.want)
			}
			for _, l := range lines[3:] {
				if strings.HasPrefix(strings.TrimSpace(ansi.Strip(l)), "ci ") {
					t.Errorf("ci listed again under the fields: %q", l)
				}
			}
		})
	}
}

// TestDetailAlign checks that every field value starts in one column sized to the longest key shown, and that a
// wrapped value continues in that column.
func TestDetailAlign(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("word ", 60))
	tests := []struct {
		name   string
		fields map[string]any
		keyW   int
	}{
		{"short keys", map[string]any{"iid": 7, "zeta": long}, 4},
		{"key longer than 12", map[string]any{"iid": 7, "open_thread_by_me": "yes", "zeta": long}, 17},
	}
	for _, tt := range tests {
		for _, width := range []int{40, 80, 200} {
			t.Run(fmt.Sprintf("%s/%d", tt.name, width), func(t *testing.T) {
				m := newNotesModel(t)
				m.width, m.height = width, 40
				m.data["p"] = []item.Item{{ID: "a", Title: "t-title", URL: "https://x/7", Author: "me", Fields: tt.fields}}
				m.rebuild()
				m.showDetail()
				col := 1 + tt.keyW + 2
				var rows, wrapped int
				for _, l := range m.detailLines()[5:] { // after the header, the blank line and the section title
					l = ansi.Strip(l)
					if n := ansi.StringWidth(l); n > width {
						t.Errorf("line %q is %d wide, screen %d", l, n, width)
					}
					if len(l) <= col || l[col-1] != ' ' || l[col] == ' ' {
						t.Errorf("value of %q not at column %d", l, col)
						continue
					}
					if strings.TrimSpace(l[:col]) == "" {
						wrapped++
					} else {
						rows++
					}
				}
				if rows != len(tt.fields) {
					t.Errorf("%d field rows, want %d", rows, len(tt.fields))
				}
				if width < 200 && wrapped == 0 {
					t.Errorf("long value did not wrap at width %d", width)
				}
				var all strings.Builder
				for _, l := range m.detailLines() {
					all.WriteString(strings.TrimSpace(ansi.Strip(l)) + " ")
				}
				if !strings.Contains(all.String(), long) {
					t.Errorf("long value truncated:\n%s", all.String())
				}
			})
		}
	}
}

// detailProv lists nothing and returns d or err as the detail of any item, counting the calls.
type detailProv struct {
	d     item.Detail
	err   error
	calls int
}

func (p *detailProv) Fetch(context.Context) ([]item.Item, error) { return nil, nil }
func (p *detailProv) Detail(context.Context, item.Item) (item.Detail, error) {
	p.calls++
	return p.d, p.err
}

// screenText is the detail screen's lines, ANSI stripped and trimmed, one per line.
func screenText(m *Model) string {
	var b strings.Builder
	for _, l := range m.detailLines() {
		b.WriteString(strings.TrimSpace(ansi.Strip(l)) + "\n")
	}
	return b.String()
}

func detailModel(t *testing.T, p *detailProv) *Model {
	t.Helper()
	m := newNotesModel(t)
	m.width, m.height = 100, 40
	if p != nil {
		m.providers["p"] = p
	}
	m.data["p"] = []item.Item{{ID: "a", Title: "t-title", URL: "https://x/7", Author: "me", Fields: map[string]any{"iid": 7}}}
	m.rebuild()
	return m
}

func TestDetailFetch(t *testing.T) {
	tests := []struct {
		name    string
		prov    *detailProv
		want    []string // substrings of the screen once the fetch landed
		absent  []string
		fetches int
	}{
		{
			name:    "sections fill in",
			prov:    &detailProv{d: item.Detail{Description: "the body", Comments: []item.Comment{{Author: "ann", Body: "lgtm"}}}},
			want:    []string{"t-title", "iid  7", "Description", "the body", "Comments", "ann", "lgtm"},
			absent:  []string{"loading"},
			fetches: 1,
		},
		{
			name:    "error shows inline",
			prov:    &detailProv{err: errors.New("boom 500")},
			want:    []string{"t-title", "iid  7", "boom 500"},
			absent:  []string{"loading", "Description"},
			fetches: 1,
		},
		{
			name:    "provider without details shows the plain fields",
			want:    []string{"t-title", "iid  7"},
			absent:  []string{"loading", "Description", "Comments"},
			fetches: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := detailModel(t, tt.prov)
			_, cmd := m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
			if m.mode != modeDetail {
				t.Fatalf("mode = %d, want detail", m.mode)
			}
			if tt.prov == nil {
				if cmd != nil {
					t.Fatal("d returned a fetch for a provider without details")
				}
			} else {
				if cmd == nil {
					t.Fatal("d started no fetch")
				}
				if s := screenText(m); !strings.Contains(s, "t-title") || !strings.Contains(s, "loading") {
					t.Errorf("before the fetch landed, want the known fields and a loading line:\n%s", s)
				}
				if tt.prov.calls != 0 {
					t.Fatal("the fetch ran on the UI goroutine")
				}
				m.Update(cmd())
			}
			s := screenText(m)
			for _, w := range tt.want {
				if !strings.Contains(s, w) {
					t.Errorf("no %q in:\n%s", w, s)
				}
			}
			for _, a := range tt.absent {
				if strings.Contains(s, a) {
					t.Errorf("%q present in:\n%s", a, s)
				}
			}
			if tt.prov != nil && tt.prov.calls != tt.fetches {
				t.Errorf("%d fetches, want %d", tt.prov.calls, tt.fetches)
			}
		})
	}
}

// TestDetailCache checks that a fetched detail is reused while the item is unchanged and refetched once its
// fingerprint changed, and that a fetch for an older version of the item is dropped.
func TestDetailCache(t *testing.T) {
	press := func(m *Model) tea.Cmd {
		_, cmd := m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
		m.mode = modeNormal
		return cmd
	}
	change := func(m *Model) {
		m.data["p"] = []item.Item{{ID: "a", Title: "new title", URL: "https://x/7", Author: "me", Fields: map[string]any{"iid": 7}}}
		m.rebuild()
	}
	t.Run("unchanged item reuses the detail", func(t *testing.T) {
		p := &detailProv{d: item.Detail{Description: "the body"}}
		m := detailModel(t, p)
		m.Update(press(m)())
		if cmd := press(m); cmd != nil {
			t.Error("d on an unchanged item fetched again")
		}
		m.showDetail()
		if s := screenText(m); !strings.Contains(s, "the body") {
			t.Errorf("cached detail not shown:\n%s", s)
		}
	})
	t.Run("changed item refetches", func(t *testing.T) {
		p := &detailProv{d: item.Detail{Description: "the body"}}
		m := detailModel(t, p)
		m.Update(press(m)())
		change(m)
		cmd := press(m)
		if cmd == nil {
			t.Fatal("d on a changed item did not refetch")
		}
		p.d.Description = "new body"
		m.Update(cmd())
		m.showDetail()
		if s := screenText(m); !strings.Contains(s, "new body") || strings.Contains(s, "the body") {
			t.Errorf("want the refetched detail only:\n%s", s)
		}
	})
	t.Run("a fetch for the old item is dropped", func(t *testing.T) {
		p := &detailProv{d: item.Detail{Description: "old body"}}
		m := detailModel(t, p)
		old := press(m)
		change(m)
		cur := press(m)
		if cur == nil {
			t.Fatal("d on a changed item did not refetch")
		}
		m.Update(old())
		m.showDetail()
		if s := screenText(m); strings.Contains(s, "old body") || !strings.Contains(s, "loading") {
			t.Errorf("a stale fetch filled the detail:\n%s", s)
		}
	})
}

func TestDetailSections(t *testing.T) {
	ago := time.Now().Add(-150 * time.Minute)
	d := item.Detail{
		Description: "line one\nline two",
		Pipeline: &item.Pipeline{ID: "42", Status: "failed", Duration: 4 * time.Minute, Jobs: []item.Job{
			{Name: "build", Stage: "build", Status: "success", Duration: 30 * time.Second},
			{Name: "lint", Stage: "test", Status: "failed", Duration: 10 * time.Second},
			{Name: "unit", Stage: "test", Status: "success", Duration: 90 * time.Second},
			{Name: "image", Stage: "build", Status: "success", Duration: 5 * time.Second},
		}},
		Threads: []item.Thread{
			{File: "main.go", Line: 12, Comments: []item.Comment{{Author: "ann", Body: "why?", Created: ago}, {Author: "bob", Body: "because", Created: ago}}},
			{Resolved: true, Comments: []item.Comment{{Author: "cy", Body: "done", Created: ago}}},
		},
		Comments: []item.Comment{
			{Author: "late", Body: "second", Created: ago.Add(time.Hour)},
			{Author: "early", Body: "first", Created: ago},
		},
	}
	m := detailModel(t, &detailProv{d: d})
	_, cmd := m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m.Update(cmd())
	s := screenText(m)
	inOrder := func(name string, subs ...string) {
		t.Helper()
		at := 0
		for _, sub := range subs {
			i := strings.Index(s[at:], sub)
			if i < 0 {
				t.Errorf("%s: no %q after offset %d in:\n%s", name, sub, at, s)
				return
			}
			at += i + len(sub)
		}
	}
	inOrder("description", "Description", "line one", "line two")
	inOrder("pipeline title", "Pipeline #42 · failed · 4m0s")
	inOrder("jobs grouped by stage", "build", "✓ build", "30s", "✓ image", "5s", "test", "✗ lint", "10s", "✓ unit", "1m30s")
	inOrder("open thread", "open · main.go:12 · ann · 2h", "why?", "bob", "because")
	inOrder("resolved thread collapsed", "resolved · cy · 2h · 0 replies", "Comments")
	if strings.Contains(s, "done") {
		t.Errorf("collapsed resolved thread shows its body:\n%s", s)
	}
	inOrder("comments newest last", "Comments", "early", "first", "late", "second")

	// GitHub checks have no stage and may have no duration; a comment may lack an author
	d = item.Detail{
		Pipeline: &item.Pipeline{ID: "9", Status: "running", Jobs: []item.Job{{Name: "ci", Status: "running"}}},
		Comments: []item.Comment{{Body: "anon", Created: ago}},
	}
	m = detailModel(t, &detailProv{d: d})
	_, cmd = m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m.Update(cmd())
	lines := strings.Split(screenText(m), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "Pipeline #9") && (i+1 >= len(lines) || lines[i+1] != "● ci") {
			t.Errorf("stageless job: want \"● ci\" right under the title, got:\n%s", strings.Join(lines, "\n"))
		}
		if strings.HasPrefix(l, "·") {
			t.Errorf("dangling separator in %q", l)
		}
	}
}

// TestDetailThreads checks the thread cursor and folding: open threads start expanded, resolved ones collapsed,
// J/K move the cursor, enter/space toggle the thread under it, za/zA fold/unfold all.
func TestDetailThreads(t *testing.T) {
	d := item.Detail{Threads: []item.Thread{
		{File: "a.go", Line: 1, Comments: []item.Comment{{Author: "ann", Body: "open-body"}, {Author: "bob", Body: "open-reply"}}},
		{File: "b.go", Line: 2, Resolved: true, Comments: []item.Comment{{Author: "cy", Body: "res-body"}, {Author: "dee", Body: "res-reply"}}},
	}}
	tests := []struct {
		name   string
		keys   []string
		want   []string // substrings of the screen
		absent []string
		cursor string // substring of the line marked with the cursor
	}{
		{"defaults", nil, []string{"open-body", "open-reply", "resolved · b.go:2 · cy · 1 reply"}, []string{"res-body", "res-reply"}, "open · a.go:1"},
		{"enter folds the open thread", []string{"enter"}, []string{"open · a.go:1 · ann · 1 reply"}, []string{"open-body", "open-reply", "res-body"}, "open · a.go:1"},
		{"J moves, enter unfolds the resolved thread", []string{"J", "enter"}, []string{"open-body", "res-body", "res-reply"}, nil, "resolved · b.go:2"},
		{"space toggles too", []string{"J", " "}, []string{"res-body"}, nil, "resolved · b.go:2"},
		{"J stops at the last thread", []string{"J", "J", "J"}, nil, nil, "resolved · b.go:2"},
		{"K moves back", []string{"J", "K", "enter"}, nil, []string{"open-body", "res-body"}, "open · a.go:1"},
		{"za folds all", []string{"z", "a"}, nil, []string{"open-body", "res-body"}, "open · a.go:1"},
		{"zA unfolds all", []string{"z", "A"}, []string{"open-body", "open-reply", "res-body", "res-reply"}, nil, "open · a.go:1"},
		{"z then another key does not fold", []string{"z", "j", "a"}, []string{"open-body"}, nil, "open · a.go:1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := detailModel(t, &detailProv{d: d})
			_, cmd := m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
			m.Update(cmd())
			for _, k := range tt.keys {
				if k == "enter" {
					m.key(tea.KeyMsg{Type: tea.KeyEnter})
					continue
				}
				nav(m, k)
			}
			if m.mode != modeDetail {
				t.Fatalf("mode = %d, want detail", m.mode)
			}
			s := screenText(m)
			for _, w := range tt.want {
				if !strings.Contains(s, w) {
					t.Errorf("no %q in:\n%s", w, s)
				}
			}
			for _, a := range tt.absent {
				if strings.Contains(s, a) {
					t.Errorf("%q present in:\n%s", a, s)
				}
			}
			var marked []string
			for _, l := range strings.Split(s, "\n") {
				if strings.HasPrefix(l, "›") {
					marked = append(marked, l)
				}
			}
			if len(marked) != 1 || !strings.Contains(marked[0], tt.cursor) {
				t.Errorf("cursor lines = %q, want one with %q", marked, tt.cursor)
			}
		})
	}
}

// TestDetailOpenThreads checks n/N: they move the thread cursor to the next/previous unresolved thread, unfold it,
// and do nothing past the last/first open thread or when every thread is resolved.
func TestDetailOpenThreads(t *testing.T) {
	th := func(file string, resolved bool) item.Thread {
		return item.Thread{File: file, Line: 1, Resolved: resolved, Comments: []item.Comment{{Author: "ann", Body: file + "-body"}}}
	}
	mixed := []item.Thread{th("a.go", false), th("b.go", true), th("c.go", false), th("d.go", true)}
	resolved := []item.Thread{th("a.go", true), th("b.go", true)}
	tests := []struct {
		name    string
		threads []item.Thread
		keys    []string
		cursor  string   // substring of the line marked with the cursor
		want    []string // substrings of the screen
	}{
		{"n skips resolved threads", mixed, []string{"n"}, "open · c.go:1", nil},
		{"n stops at the last open thread", mixed, []string{"n", "n"}, "open · c.go:1", nil},
		{"n after the last open thread does nothing", mixed, []string{"J", "J", "J", "n"}, "resolved · d.go:1", nil},
		{"N skips resolved threads", mixed, []string{"J", "J", "J", "N"}, "open · c.go:1", nil},
		{"N goes back", mixed, []string{"n", "N"}, "open · a.go:1", nil},
		{"N at the first open thread does nothing", mixed, []string{"N"}, "open · a.go:1", nil},
		{"N from a resolved thread", mixed, []string{"J", "N"}, "open · a.go:1", nil},
		{"n unfolds a folded open thread", mixed, []string{"J", "J", "enter", "K", "K", "n"}, "open · c.go:1", []string{"c.go-body"}},
		{"n with every thread resolved does nothing", resolved, []string{"n"}, "resolved · a.go:1", nil},
		{"N with every thread resolved does nothing", resolved, []string{"J", "N"}, "resolved · b.go:1", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := detailModel(t, &detailProv{d: item.Detail{Threads: tt.threads}})
			_, cmd := m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
			m.Update(cmd())
			for _, k := range tt.keys {
				if k == "enter" {
					m.key(tea.KeyMsg{Type: tea.KeyEnter})
					continue
				}
				nav(m, k)
			}
			if m.mode != modeDetail {
				t.Fatalf("mode = %d, want detail", m.mode)
			}
			s := screenText(m)
			var marked []string
			for _, l := range strings.Split(s, "\n") {
				if strings.HasPrefix(l, "›") {
					marked = append(marked, l)
				}
			}
			if len(marked) != 1 || !strings.Contains(marked[0], tt.cursor) {
				t.Errorf("cursor lines = %q, want one with %q", marked, tt.cursor)
			}
			for _, w := range tt.want {
				if !strings.Contains(s, w) {
					t.Errorf("no %q in:\n%s", w, s)
				}
			}
		})
	}
}

// TestDetailOpenThreadsScroll checks that n and N scroll the thread they select into view.
func TestDetailOpenThreadsScroll(t *testing.T) {
	d := item.Detail{Threads: []item.Thread{
		{File: "a.go", Line: 1, Comments: []item.Comment{{Author: "ann", Body: strings.Repeat("long\n", 20) + "a-end"}}},
		{File: "b.go", Line: 2, Comments: []item.Comment{{Author: "cy", Body: "b-body"}}},
	}}
	m := detailModel(t, &detailProv{d: d})
	m.height = 8
	_, cmd := m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m.Update(cmd())
	nav(m, "n")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "open · b.go:2") {
		t.Errorf("n did not scroll the thread into view:\n%s", v)
	}
	nav(m, "G", "N")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "open · a.go:1") {
		t.Errorf("N did not scroll the thread into view:\n%s", v)
	}
}

// TestDetailThreadsKeep checks that folding survives a refetched detail of the changed item and that J scrolls
// the thread under the cursor into view.
func TestDetailThreadsKeep(t *testing.T) {
	th := func(body string) item.Detail {
		return item.Detail{Description: strings.Repeat("desc\n", 20) + "end", Threads: []item.Thread{
			{File: "a.go", Line: 1, Comments: []item.Comment{{Author: "ann", Body: "open-body"}}},
			{File: "b.go", Line: 2, Resolved: true, Comments: []item.Comment{{Author: "cy", Body: body}}},
		}}
	}
	p := &detailProv{d: th("res-body")}
	m := detailModel(t, p)
	m.height = 8
	_, cmd := m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m.Update(cmd())
	nav(m, "J")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "resolved · b.go:2") {
		t.Errorf("J did not scroll the thread into view:\n%s", v)
	}
	m.key(tea.KeyMsg{Type: tea.KeyEnter})
	nav(m, "q")
	m.data["p"] = []item.Item{{ID: "a", Title: "new title", URL: "https://x/7", Author: "me", Fields: map[string]any{"iid": 7}}}
	m.rebuild()
	p.d = th("res-body-2")
	_, cmd = m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if cmd == nil {
		t.Fatal("d on a changed item did not refetch")
	}
	m.Update(cmd())
	if s := screenText(m); !strings.Contains(s, "res-body-2") || !strings.Contains(s, "open-body") {
		t.Errorf("folding lost on refresh, want the resolved thread still unfolded:\n%s", s)
	}
}
