package ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/config"
)

// groupModel is a model whose only view has the given groups and sort, filled with items "a".."f" whose n field is
// 1..6 (so "a" has n 1).
func groupModel(t *testing.T, groups []config.Group, sort string) *Model {
	t.Helper()
	m := newBackoffModel(t)
	m.cfg.View[0].Groups, m.cfg.View[0].Sort = groups, sort
	m.applyConfig(m.cfg)
	its := mkItems("a", "b", "c", "d", "e", "f")
	for i := range its {
		its[i].Fields["n"] = i + 1
	}
	m.Update(fetchedMsg{name: "p", items: its})
	return m
}

// listLines renders the list and returns one token per non-empty line: the header text or the row's item ID.
func listLines(m *Model) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimRight(ansi.Strip(m.list()), "\n"), "\n") {
		l = strings.TrimSpace(l)
		switch {
		case l == "":
		case strings.HasPrefix(l, "──"):
			out = append(out, l)
		default:
			out = append(out, strings.Fields(strings.TrimPrefix(l, "▌"))[0]) // ▌ marks a marked row
		}
	}
	return out
}

func TestGroupedList(t *testing.T) {
	tests := []struct {
		name   string
		groups []config.Group
		sort   string
		want   []string // listLines
	}{
		{"no groups: flat list as before", nil, "", []string{"a", "b", "c", "d", "e", "f"}},
		{
			"groups in config order, ungrouped tail without header",
			[]config.Group{{Name: "high", Filter: "n >= 5"}, {Name: "low", Filter: "n <= 2"}}, "",
			[]string{"── high (2) ──", "e", "f", "── low (2) ──", "a", "b", "c", "d"},
		},
		{
			"an item matching several groups appears only in the first",
			[]config.Group{{Name: "even", Filter: "n % 2 == 0"}, {Name: "big", Filter: "n > 3"}}, "",
			[]string{"── even (3) ──", "b", "d", "f", "── big (1) ──", "e", "a", "c"},
		},
		{
			"empty group is hidden",
			[]config.Group{{Name: "none", Filter: "n > 100"}, {Name: "one", Filter: `id == "c"`}}, "",
			[]string{"── one (1) ──", "c", "a", "b", "d", "e", "f"},
		},
		{
			"view sort applies inside groups and the tail",
			[]config.Group{{Name: "odd", Filter: "n % 2 == 1"}}, "-n",
			[]string{"── odd (3) ──", "e", "c", "a", "f", "d", "b"},
		},
		{
			"every item grouped: no tail",
			[]config.Group{{Name: "all"}}, "",
			[]string{"── all (6) ──", "a", "b", "c", "d", "e", "f"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := groupModel(t, tt.groups, tt.sort)
			if got := listLines(m); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("list = %q, want %q", got, tt.want)
			}
			var ids []string
			for _, l := range tt.want {
				if !strings.HasPrefix(l, "──") {
					ids = append(ids, l)
				}
			}
			if got := itemIDs(m.items()); !reflect.DeepEqual(got, ids) {
				t.Errorf("items = %v, want %v (cursor order skips headers)", got, ids)
			}
		})
	}
}

func TestGroupFilterErrorSurfaces(t *testing.T) {
	m := groupModel(t, []config.Group{{Name: "bad", Filter: "title > 2"}}, "")
	if len(m.items()) != 0 {
		t.Errorf("items = %v, want none while the group filter fails", itemIDs(m.items()))
	}
	if got := ansi.Strip(m.list()); !strings.Contains(got, "filter error") || !strings.Contains(got, `"bad"`) {
		t.Errorf("list = %q, want a filter error naming group \"bad\"", got)
	}
}

func TestGroupedScroll(t *testing.T) {
	groups := []config.Group{{Name: "x", Filter: "n <= 3"}, {Name: "y", Filter: "n >= 4"}}
	// lines: 0 header x, 1 a, 2 b, 3 c, 4 header y, 5 d, 6 e, 7 f
	tests := []struct {
		name string
		keys []string
		want []string // visible lines, list height 4
	}{
		{"start shows the first header", nil, []string{"── x (3) ──", "a", "b", "c"}},
		{"j past the page keeps the cursor row visible", []string{"j", "j", "j"}, []string{"b", "c", "── y (3) ──", "d"}},
		{"G shows the last item", []string{"G"}, []string{"── y (3) ──", "d", "e", "f"}},
		{"gg after G shows the first header again", []string{"G", "g", "g"}, []string{"── x (3) ──", "a", "b", "c"}},
		{"k onto a group's first item shows its header", []string{"G", "k", "k", "k", "k", "k"}, []string{"── x (3) ──", "a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := groupModel(t, groups, "")
			m.height = 8 // list height 4
			for i := range tt.keys {
				nav(m, tt.keys[i])
				m.list() // render after every key, as the app does
			}
			got := listLines(m)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("visible = %q, want %q", got, tt.want)
			}
			if sel := m.selectedID(); !contains(got, sel) {
				t.Errorf("cursor item %q not visible in %q", sel, got)
			}
		})
	}
}

func TestGroupedCursorSurvivesRefresh(t *testing.T) {
	m := groupModel(t, []config.Group{{Name: "high", Filter: "n >= 4"}}, "")
	nav(m, "j", " ") // items: d e f a b c → mark e, cursor on f
	if m.selectedID() != "f" {
		t.Fatalf("setup: selected %q", m.selectedID())
	}
	its := mkItems("f", "x", "a")
	its[0].Fields["n"], its[1].Fields["n"], its[2].Fields["n"] = 6, 9, 1
	m.Update(fetchedMsg{name: "p", items: its})
	if got := m.selectedID(); got != "f" {
		t.Errorf("cursor on %q, want f", got)
	}
	if !m.marks["e"] {
		t.Error("mark on e lost")
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
