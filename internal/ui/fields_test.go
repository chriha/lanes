package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/history"
	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/session"
)

func TestPreviewShowsIdentifier(t *testing.T) {
	tests := []struct {
		key  string
		val  any
		want string
	}{
		{"iid", "7", "iid         7"},
		{"number", 12, "number      12"},
		{"key", "EQD-1010", "key         EQD-1010"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			m := newNotesModel(t)
			// labels is a preferred field, so the preview lists only preferred ones
			m.data["p"] = []item.Item{{ID: "a", Fields: map[string]any{tt.key: tt.val, "labels": []string{"bug"}, "kind": "pr"}}}
			m.rebuild()
			got := ansi.Strip(m.preview())
			if !strings.Contains(got, tt.want) {
				t.Errorf("preview lacks %q:\n%s", tt.want, got)
			}
			if strings.Contains(got, "kind") {
				t.Errorf("preview lists a non-preferred field:\n%s", got)
			}
		})
	}
}

// fieldsModel has a command provider with fetched items, one never fetched, and a gitlab provider.
func fieldsModel(t *testing.T, width int) *Model {
	t.Helper()
	m := newNotesModel(t)
	m.cfg.Provider = map[string]config.Provider{
		"p":     {Type: "command", Command: "true"},
		"later": {Type: "command", Command: "true"},
		"gl":    {Type: "gitlab", Project: "g/p"},
	}
	m.data["p"] = []item.Item{
		{ID: "a", Fields: map[string]any{"foo": 1, "bar": "x"}},
		{ID: "b", Fields: map[string]any{"baz": true, "foo": 2}},
	}
	m.fetchedAt["p"] = time.Now()
	m.rebuild()
	m.width = width
	return m
}

// fieldsSection returns the help lines from the fields title on, ANSI stripped.
func fieldsSection(t *testing.T, m *Model) []string {
	t.Helper()
	lines := m.helpLines()
	for i, l := range lines {
		if f := strings.Fields(ansi.Strip(l)); len(f) > 0 && f[0] == "fields" {
			out := make([]string, 0, len(lines)-i)
			for _, l := range lines[i:] {
				out = append(out, ansi.Strip(l))
			}
			return out
		}
	}
	t.Fatalf("help has no fields section:\n%s", ansi.Strip(strings.Join(lines, "\n")))
	return nil
}

func TestHelpFieldsSection(t *testing.T) {
	m := fieldsModel(t, 200)
	sec := fieldsSection(t, m)
	text := strings.Join(sec, "\n")
	if strings.Contains(text, "state ") {
		t.Errorf("fields section is not at the bottom (the state path follows it):\n%s", text)
	}
	for _, want := range []struct{ label, keys string }{
		{"common", ".id .title .url .author .draft .note"},
		{"gitlab", ".approved .approved_by .approved_by_me .assignees .branch .ci .conflicts .diverged .iid .kind"},
		{"github", ".me .number .open_thread_by_me"},
		{"github-issues", ".assignees .blocked .blocked_by .blocking .closed_links"},
		{"linear", ".assignees .blocked .blocked_by .blocking .closed_links"},
		{"p (command)", ".bar .baz .foo"},
		{"later (command)", "fetch pending"},
		{"actions only", ".settings .session .action .vars"},
	} {
		found := false
		for _, l := range sec {
			if strings.HasPrefix(strings.TrimSpace(l), want.label+" ") && strings.Contains(l, want.keys) {
				found = true
			}
		}
		if !found {
			t.Errorf("fields section lacks a %q line with %q:\n%s", want.label, want.keys, text)
		}
	}
	if strings.Contains(text, "gl (command)") || strings.Contains(text, "auto") {
		t.Errorf("fields section lists a built-in provider as a command provider or auto:\n%s", text)
	}
	if last := sec[len(sec)-1]; !strings.Contains(last, "actions only") {
		t.Errorf("last help line = %q, want the actions only line", last)
	}
}

func TestHelpFieldsWrap(t *testing.T) {
	const width = 60
	m := fieldsModel(t, width)
	sec := fieldsSection(t, m)
	indent := -1
	for i, l := range sec {
		if w := lipgloss.Width(l); w > width {
			t.Errorf("line %d is %d wide, terminal is %d: %q", i, w, width, l)
		}
		if strings.HasPrefix(strings.TrimSpace(l), "gitlab ") {
			indent = strings.Index(l, ".approved")
			// the gitlab keys do not fit in 60 columns: the next line continues them, indented under the keys
			if i+1 >= len(sec) {
				t.Fatalf("no line after the gitlab line")
			}
			next := sec[i+1]
			if !strings.HasPrefix(next, strings.Repeat(" ", indent)+".") {
				t.Errorf("continuation %q is not indented to column %d", next, indent)
			}
		}
	}
	if indent < 0 {
		t.Fatalf("no gitlab line:\n%s", strings.Join(sec, "\n"))
	}
	joined := strings.Join(strings.Fields(strings.Join(sec, " ")), " ")
	if !strings.Contains(joined, ".target .ticket") || strings.Contains(joined, "…") {
		t.Errorf("wrapped keys are lost or truncated:\n%s", strings.Join(sec, "\n"))
	}
}

func TestPreviewShowsRelations(t *testing.T) {
	tests := []struct {
		name   string
		fields map[string]any
		want   []string
		absent []string
	}{
		{"open and closed", map[string]any{
			"number": 7, "labels": []string{"bug"}, "assignees": []string{"me"},
			"blocked_by": []string{"#7", "#8"}, "blocking": []string{}, "related": []string{"#1"},
			"closed_links": []string{"blocked_by:#9", "related:#12", "related:#13"},
		}, []string{"blocked_by  #7, #8, #9 ✓", "related     #1, #12 ✓, #13 ✓"}, []string{"blocking", "closed_links"}},
		{"only closed, decoded from JSON", map[string]any{
			"number": 7, "labels": []string{"bug"},
			"blocked_by": []any{}, "blocking": []any{}, "related": []any{},
			"closed_links": []any{"blocking:#3"},
		}, []string{"blocking    #3 ✓"}, []string{"blocked_by", "related", "closed_links"}},
		{"none", map[string]any{
			"number": 7, "labels": []string{"bug"},
			"blocked_by": []string{}, "blocking": []string{}, "related": []string{}, "closed_links": []string{},
		}, nil, []string{"blocked_by", "blocking", "related", "closed_links"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newNotesModel(t)
			m.data["p"] = []item.Item{{ID: "a", Fields: tt.fields}}
			m.rebuild()
			got := ansi.Strip(m.preview())
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("preview lacks %q:\n%s", w, got)
				}
			}
			for _, a := range tt.absent {
				if strings.Contains(got, a) {
					t.Errorf("preview has %q:\n%s", a, got)
				}
			}
		})
	}
}

func TestPreviewKeepsSessionAndLastRunLines(t *testing.T) {
	full := map[string]any{
		"number": 7, "branch": "feat/x", "labels": []string{"bug"}, "reviewers": []string{"r"}, "assignees": []string{"me"},
		"blocked_by": []string{"#1"}, "blocking": []string{"#2"}, "related": []string{"#3"},
	}
	tests := []struct {
		name    string
		fields  map[string]any
		session bool
		run     bool
		want    []string
	}{
		{"all relations, session and run", full, true, true, []string{"session", "last run"}},
		{"all relations, session only", full, true, false, []string{"session"}},
		{"all relations, run only", full, false, true, []string{"last run"}},
		{"few fields", map[string]any{"number": 7}, true, true, []string{"session", "last run"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newNotesModel(t)
			m.data["p"] = []item.Item{{ID: "a", Fields: tt.fields}}
			m.rebuild()
			if tt.session {
				m.sessByItem[session.Key("a")] = []session.Info{{Name: "s", ItemKey: session.Key("a"), Created: time.Now()}}
			}
			if tt.run {
				history.Append(history.Event{Item: session.Key("a"), Action: "work", Event: "finished"})
			}
			got := ansi.Strip(m.preview())
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("preview lacks %q:\n%s", w, got)
				}
			}
			if n := strings.Count(got, "\n"); n != previewH {
				t.Errorf("preview has %d rows, want %d:\n%s", n, previewH, got)
			}
		})
	}
}
