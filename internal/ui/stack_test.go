package ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/seen"
)

// stackItems is a stack of three (c on b on a) and an item in no stack (d); listed top first, so the provider
// order is not the stack order.
func stackItems() []item.Item {
	mr := func(id, branch, target string) item.Item {
		return item.Item{ID: id, Title: "title " + id, Fields: map[string]any{"branch": branch, "target": target}}
	}
	return []item.Item{mr("c", "f3", "f2"), mr("b", "f2", "f1"), mr("a", "f1", "main"), mr("d", "g", "main")}
}

// newStackModel lists stackItems under one view per given view config.
func newStackModel(t *testing.T, views ...config.View) *Model {
	t.Helper()
	stackEnv(t)
	return startStackModel(views...)
}

// stackEnv points the config, state, cache and data dirs at fresh temp dirs.
func stackEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "HOME"} {
		t.Setenv(k, t.TempDir())
	}
}

// startStackModel builds the model on the current environment, as a restart would.
func startStackModel(views ...config.View) *Model {
	for i := range views {
		views[i].Provider = "p"
	}
	m := New(&config.Config{
		Provider: map[string]config.Provider{"p": {Type: "command", Command: "true"}},
		View:     views,
	})
	m.width, m.height = 120, 40
	m.data["p"] = stackItems()
	m.rebuild()
	return m
}

func TestStackFields(t *testing.T) {
	tests := []struct {
		name string
		view config.View
		want []string
	}{
		{"base", config.View{Filter: `base == "a"`}, []string{"b"}},
		{"stack_pos", config.View{Filter: `stack_pos == 2`}, []string{"b"}},
		{"stack_size", config.View{Filter: `stack_size == 3`}, []string{"c", "b", "a"}},
		{"stacked_by", config.View{Filter: `"c" in stacked_by`}, []string{"b"}},
		{"in no stack", config.View{Filter: `stack_pos == 0 && base == "" && stack_size == 0 && len(stacked_by) == 0`}, []string{"d"}},
		{"sort by stack_pos", config.View{Filter: `stack_pos > 0`, Sort: "stack_pos"}, []string{"a", "b", "c"}},
		{"group", config.View{Filter: `true`, Groups: []config.Group{{Name: "bottom", Filter: `stack_pos == 1`}}}, []string{"a", "c", "b", "d"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.view.Name = "v"
			m := newStackModel(t, tt.view)
			if err := m.views[0].err; err != nil {
				t.Fatalf("view error: %v", err)
			}
			if got := itemIDs(m.items()); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("items = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStackFieldsWithoutBranches(t *testing.T) {
	m := newStackModel(t, config.View{Name: "v", Filter: `stack_pos == 0 && base == "" && len(stacked_by) == 0`, Sort: "stack_size"})
	m.data["p"] = mkItems("t1", "t2") // tickets: no branch, no target
	m.rebuild()
	if err := m.views[0].err; err != nil {
		t.Fatalf("view error: %v", err)
	}
	if got := itemIDs(m.items()); !reflect.DeepEqual(got, []string{"t1", "t2"}) {
		t.Errorf("items = %v, want [t1 t2]", got)
	}
}

func TestStackProviderFieldWins(t *testing.T) {
	m := newStackModel(t, config.View{Name: "v", Filter: `true`})
	its := stackItems()
	its[1].Fields["base"] = "provider" // b
	m.data["p"] = its
	m.rebuild()
	if got := itemOf(t, m, "b").Env()["base"]; got != "provider" {
		t.Errorf("base = %v, want the provider's value", got)
	}
}

func TestStackRow(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(0) // termenv.TrueColor: render the dim the test looks for
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	tests := []struct {
		name     string
		id       string
		wantMark string // "" = no marker
	}{
		{"on a base hidden by the view's filter", "b", "↑ a"},
		{"on a listed base", "c", "↑ b"},
		{"in no stack", "d", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newStackModel(t, config.View{Name: "v", Filter: `id != "a"`})
			line := m.row(itemOf(t, m, tt.id), false, idColWidth(m.items()))
			plain := ansi.Strip(line)
			title := "title " + tt.id
			if dim := strings.Contains(line, sDim.Render(title)); dim != (tt.wantMark != "") {
				t.Errorf("dim title = %v, want %v in %q", dim, tt.wantMark != "", line)
			}
			if tt.wantMark == "" {
				if strings.Contains(plain, "↑") {
					t.Errorf("row = %q, has a stack marker", plain)
				}
			} else if !strings.Contains(plain, title+" "+tt.wantMark) {
				t.Errorf("row = %q, want %q after the title", plain, tt.wantMark)
			}
			if got, want := ansi.StringWidth(line), m.width-1; got != want {
				t.Errorf("row width %d, want %d", got, want)
			}
		})
	}
}

func TestStackRowNarrowKeepsWidth(t *testing.T) {
	m := newStackModel(t, config.View{Name: "v", Filter: `true`})
	m.width = 40
	its := m.data["p"]
	its[1].Title = strings.Repeat("long title ", 10) // b, stacked on a
	m.rebuild()
	line := m.row(itemOf(t, m, "b"), false, idColWidth(m.items()))
	if got, want := ansi.StringWidth(line), m.width-1; got != want {
		t.Errorf("row width %d, want %d: %q", got, want, ansi.Strip(line))
	}
	if !strings.Contains(ansi.Strip(line), "↑ a") {
		t.Errorf("row = %q, lost the stack marker", ansi.Strip(line))
	}
}

func TestStackChangeKeepsItemRead(t *testing.T) {
	stackEnv(t)
	if err := (seen.Store{}).MarkRead(map[string]string{"x": "old"}); err != nil {
		t.Fatal(err) // a seen.json from an earlier run: items can be unread
	}
	m := startStackModel(config.View{Name: "v", Filter: `true`})
	m.markRead(itemOf(t, m, "b")) // read while stacked on a
	if m.isUnread("p", m.data["p"][1]) {
		t.Fatal("b unread right after marking it read")
	}
	its := stackItems()
	m.data["p"] = []item.Item{its[0], its[1], its[3]} // a merged: b is the bottom of the stack now
	m.rebuild()
	got := itemOf(t, m, "b")
	if got.Env()["base"] != "" {
		t.Fatalf("base = %v after a merged, want empty", got.Env()["base"])
	}
	if got.Env()["unread"] != false {
		t.Error("b turned unread although only its stack changed")
	}
}
