package ui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/cache"
	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/item"
)

func mkItems(ids ...string) []item.Item {
	out := make([]item.Item, len(ids))
	for i, id := range ids {
		out[i] = item.Item{ID: id, Title: id, Fields: map[string]any{}}
	}
	return out
}

func itemIDs(its []item.Item) []string {
	out := make([]string, len(its))
	for i, it := range its {
		out[i] = it.ID
	}
	return out
}

func nav(m *Model, keys ...string) {
	for _, k := range keys {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		switch k {
		case "ctrl+d":
			msg = tea.KeyMsg{Type: tea.KeyCtrlD}
		case "ctrl+u":
			msg = tea.KeyMsg{Type: tea.KeyCtrlU}
		case " ":
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
		}
		m.key(msg)
	}
}

func TestInherit(t *testing.T) {
	withField := func(id, f string, v any, pending ...string) item.Item {
		return item.Item{ID: id, Fields: map[string]any{f: v}, Pending: pending}
	}
	tests := []struct {
		name       string
		old, fresh []item.Item
		wantField  any // fresh[0].Fields["ci"]
	}{
		{"pending field is carried over", []item.Item{withField("a", "ci", "pass")}, []item.Item{withField("a", "ci", nil, "ci")}, "pass"},
		{"non-pending field is not overwritten", []item.Item{withField("a", "ci", "pass")}, []item.Item{withField("a", "ci", "fail")}, "fail"},
		{"pending but unknown before stays unset", []item.Item{withField("a", "other", 1)}, []item.Item{{ID: "a", Fields: map[string]any{}, Pending: []string{"ci"}}}, nil},
		{"new item is untouched", []item.Item{withField("b", "ci", "pass")}, []item.Item{withField("a", "ci", "x", "ci")}, "x"},
		{"empty snapshot", nil, []item.Item{withField("a", "ci", "x", "ci")}, "x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inherit(tt.old, tt.fresh)
			if v := got[0].Fields["ci"]; v != tt.wantField {
				t.Errorf("ci = %v, want %v", v, tt.wantField)
			}
		})
	}
}

func TestFetchedMsg(t *testing.T) {
	boom := errors.New("boom")
	t.Run("full result stores items, stamps time, clears loading", func(t *testing.T) {
		m := newBackoffModel(t)
		m.loading["p"] = true
		m.Update(fetchedMsg{name: "p", items: mkItems("a", "b")})
		if got := itemIDs(m.items()); !reflect.DeepEqual(got, []string{"a", "b"}) {
			t.Errorf("items = %v", got)
		}
		if m.loading["p"] || m.fetchedAt["p"].IsZero() {
			t.Errorf("loading=%v fetchedAt=%v", m.loading["p"], m.fetchedAt["p"])
		}
	})
	t.Run("partial result shows items but keeps loading and no timestamp", func(t *testing.T) {
		m := newBackoffModel(t)
		m.loading["p"] = true
		m.Update(fetchedMsg{name: "p", items: mkItems("a"), partial: true})
		if len(m.items()) != 1 {
			t.Errorf("items = %v", itemIDs(m.items()))
		}
		if !m.loading["p"] || !m.fetchedAt["p"].IsZero() {
			t.Errorf("loading=%v fetchedAt=%v", m.loading["p"], m.fetchedAt["p"])
		}
	})
	t.Run("partial result inherits pending fields from the previous full one", func(t *testing.T) {
		m := newBackoffModel(t)
		m.Update(fetchedMsg{name: "p", items: []item.Item{{ID: "a", Fields: map[string]any{"ci": "pass"}}}})
		m.Update(fetchedMsg{name: "p", partial: true, items: []item.Item{{ID: "a", Fields: map[string]any{}, Pending: []string{"ci"}}}})
		if v := m.items()[0].Fields["ci"]; v != "pass" {
			t.Errorf("ci = %v, want pass", v)
		}
	})
	t.Run("error sets provErr and keeps items", func(t *testing.T) {
		m := newBackoffModel(t)
		m.Update(fetchedMsg{name: "p", items: mkItems("a")})
		m.loading["p"] = true
		m.Update(fetchedMsg{name: "p", err: boom})
		if m.provErr["p"] != boom || m.fails["p"] != 1 || m.loading["p"] {
			t.Errorf("provErr=%v fails=%d loading=%v", m.provErr["p"], m.fails["p"], m.loading["p"])
		}
		if len(m.items()) != 1 {
			t.Errorf("items lost on error: %v", itemIDs(m.items()))
		}
	})
	t.Run("partial error is ignored while loading continues", func(t *testing.T) {
		m := newBackoffModel(t)
		m.loading["p"] = true
		m.Update(fetchedMsg{name: "p", err: boom, partial: true})
		if m.provErr["p"] != nil || m.fails["p"] != 0 || !m.loading["p"] {
			t.Errorf("provErr=%v fails=%d loading=%v", m.provErr["p"], m.fails["p"], m.loading["p"])
		}
	})
	t.Run("success clears provErr", func(t *testing.T) {
		m := newBackoffModel(t)
		m.Update(fetchedMsg{name: "p", err: boom})
		m.Update(fetchedMsg{name: "p", items: mkItems("a")})
		if m.provErr["p"] != nil || m.fails["p"] != 0 {
			t.Errorf("provErr=%v fails=%d", m.provErr["p"], m.fails["p"])
		}
	})
	t.Run("partial success clears provErr but not the backoff", func(t *testing.T) {
		m := newBackoffModel(t)
		m.Update(fetchedMsg{name: "p", err: boom})
		m.Update(fetchedMsg{name: "p", items: mkItems("a"), partial: true})
		if m.provErr["p"] != nil || m.fails["p"] != 1 {
			t.Errorf("provErr=%v fails=%d", m.provErr["p"], m.fails["p"])
		}
	})
}

func TestRefreshKeepsCursorAndMarks(t *testing.T) {
	tests := []struct {
		name       string
		refreshed  []string
		wantCursor string
		wantMarked []string
	}{
		{"item added above", []string{"x", "a", "b", "c", "d"}, "c", []string{"b", "d"}},
		{"item removed above", []string{"b", "c", "d"}, "c", []string{"b", "d"}},
		{"reordered", []string{"d", "c", "b", "a"}, "c", []string{"d", "b"}},
		{"cursor item removed", []string{"a", "b", "d"}, "d", []string{"b", "d"}}, // index 2 survives
		{"cursor item removed at end", []string{"a", "b"}, "b", []string{"b"}},
		{"marked item removed", []string{"a", "c", "d"}, "c", []string{"d"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBackoffModel(t)
			m.Update(fetchedMsg{name: "p", items: mkItems("a", "b", "c", "d")})
			nav(m, "j", " ") // mark b, cursor on c
			m.marks["d"] = true
			if m.selectedID() != "c" {
				t.Fatalf("setup: selected %q", m.selectedID())
			}
			m.Update(fetchedMsg{name: "p", items: mkItems(tt.refreshed...)})
			if got := m.selectedID(); got != tt.wantCursor {
				t.Errorf("cursor on %q, want %q", got, tt.wantCursor)
			}
			got := map[string]bool{}
			for id := range m.marks {
				got[id] = true
			}
			// marks are keyed by ID and are not pruned when an item disappears
			want := map[string]bool{"b": true, "d": true}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("marks = %v, want %v", got, want)
			}
			var visible []string
			for _, it := range m.items() {
				if m.marks[it.ID] {
					visible = append(visible, it.ID)
				}
			}
			if !reflect.DeepEqual(visible, tt.wantMarked) && (len(visible) != 0 || len(tt.wantMarked) != 0) {
				t.Errorf("visible marked = %v, want %v", visible, tt.wantMarked)
			}
		})
	}
}

func TestNavigationClamps(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want int
	}{
		{"j moves down", []string{"j"}, 1},
		{"j clamps at the end", []string{"j", "j", "j", "j", "j", "j", "j"}, 4},
		{"k clamps at the start", []string{"k"}, 0},
		{"k moves up", []string{"j", "j", "k"}, 1},
		{"G goes to the end", []string{"G"}, 4},
		{"gg goes to the start", []string{"G", "g", "g"}, 0},
		{"single g does nothing", []string{"G", "g"}, 4},
		{"g interrupted by k starts over", []string{"G", "g", "k", "g"}, 3},
		{"ctrl+d clamps at the end", []string{"ctrl+d"}, 4},
		{"ctrl+u clamps at the start", []string{"j", "ctrl+u"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBackoffModel(t)
			m.Update(fetchedMsg{name: "p", items: mkItems("a", "b", "c", "d", "e")})
			nav(m, tt.keys...)
			if m.cursor != tt.want {
				t.Errorf("cursor = %d, want %d", m.cursor, tt.want)
			}
		})
	}
	t.Run("half page moves by listHeight/2", func(t *testing.T) {
		m := newBackoffModel(t)
		m.height = 8 // list height 4, half page 2
		m.Update(fetchedMsg{name: "p", items: mkItems("a", "b", "c", "d", "e", "f")})
		nav(m, "ctrl+d")
		if m.cursor != 2 {
			t.Errorf("after ctrl+d cursor = %d, want 2", m.cursor)
		}
		nav(m, "ctrl+u")
		if m.cursor != 0 {
			t.Errorf("after ctrl+u cursor = %d, want 0", m.cursor)
		}
	})
	t.Run("empty list", func(t *testing.T) {
		m := newBackoffModel(t)
		nav(m, "j", "G", "ctrl+d", "k")
		if m.cursor != 0 {
			t.Errorf("cursor = %d, want 0", m.cursor)
		}
	})
}

func TestUsageText(t *testing.T) {
	now := time.Unix(10_000, 0)
	tests := []struct {
		name string
		u    usageMsg
		want string
	}{
		{"nothing configured", usageMsg{}, ""},
		{"fresh is shown as is", usageMsg{"5h 23%", now.Add(-time.Minute)}, "claude usage: 5h 23%"},
		{"stale carries its age", usageMsg{"5h 23%", now.Add(-12 * time.Minute)}, "claude usage: 5h 23% (12m ago)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Model{usage: tt.u}
			if got := m.usageText(now); got != tt.want {
				t.Errorf("usageText = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFooterUsage(t *testing.T) {
	tests := []struct {
		name   string
		notice string
		want   bool
	}{
		{"shown at the left of the footer", "", true},
		{"a notice takes its place", "config reloaded", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBackoffModel(t)
			m.width = 160
			m.notice = tt.notice
			m.Update(usageMsg{"5h 23% · wk 30%", time.Now()})
			f := ansi.Strip(m.footer())
			if got := strings.HasPrefix(f, " claude usage: 5h 23% · wk 30%"); got != tt.want {
				t.Errorf("footer = %q, usage at start = %v, want %v", f, got, tt.want)
			}
		})
	}
}

func TestIDColumn(t *testing.T) {
	long := strings.Repeat("x", 25)
	tests := []struct {
		name    string
		ids     []string
		wantW   int
		wantID  string // the ID cell as rendered for ids[0]
		wantEnd string // rendered ID cell for the last id
	}{
		{"all short", []string{"!1", "!123"}, 7, "!1     ", "!123   "},
		{"eight chars", []string{"EQD-1001"}, 8, "EQD-1001", "EQD-1001"},
		{"mix, longest wins", []string{"EQD-1", "EQD-1001", "!7"}, 8, "EQD-1  ", "!7      "},
		{"over the max", []string{long, "!1"}, 20, strings.Repeat("x", 19) + "…", "!1                  "},
		{"empty view", nil, 7, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			its := mkItems(tt.ids...)
			if got := idColWidth(its); got != tt.wantW {
				t.Fatalf("idColWidth = %d, want %d", got, tt.wantW)
			}
			if len(its) == 0 {
				return
			}
			m := newNotesModel(t)
			for i, want := range map[int]string{0: tt.wantID, len(its) - 1: tt.wantEnd} {
				line := ansi.Strip(m.row(its[i], false, tt.wantW))
				if !strings.HasPrefix(line[3:], want) {
					t.Errorf("row %d = %q, want ID cell %q", i, line, want)
				}
				// the title column absorbs the ID width: rows keep the width of the 7-wide layout
				if w := ansi.StringWidth(line); w != m.width-1 {
					t.Errorf("row %d is %d cells wide, want %d", i, w, m.width-1)
				}
			}
		})
	}
}

func TestRowAuthorFollowsTitle(t *testing.T) {
	long := strings.Repeat("a", 30)
	tests := []struct {
		name   string
		title  string
		author string
		badges []item.Badge
		// wantPrefix is what the row must contain right after the ID cell and its separator
		wantPrefix string
	}{
		{"short title", "fix it", "alice", []item.Badge{{Text: "ok"}}, "fix it  alice  "},
		{"no author", "fix it", "", []item.Badge{{Text: "ok"}}, "fix it   "},
		{"long author is cut", "fix it", long, nil, "fix it  " + strings.Repeat("a", 15) + "…"},
		{"title cut first", strings.Repeat("t", 200), "alice", []item.Badge{{Text: "ok"}}, strings.Repeat("t", 57) + "…  alice "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newNotesModel(t)
			m.width = 80
			it := item.Item{ID: "!1", Title: tt.title, Author: tt.author, Badges: tt.badges, Fields: map[string]any{}}
			line := ansi.Strip(m.row(it, false, 7))
			cell := line[3+7+1:]
			if !strings.HasPrefix(cell, tt.wantPrefix) {
				t.Errorf("row = %q, want %q after the ID", line, tt.wantPrefix)
			}
			if tt.author == "" && !reflect.DeepEqual(strings.Fields(cell), []string{"fix", "it", "ok"}) {
				t.Errorf("row = %q, dangling content after the title", line)
			}
			if w := ansi.StringWidth(line); w != m.width-1 {
				t.Errorf("row is %d cells wide, want %d", w, m.width-1)
			}
			if len(tt.badges) > 0 && !strings.HasSuffix(strings.TrimRight(line, " "), "ok") {
				t.Errorf("row = %q, badges not at the right edge", line)
			}
		})
	}
}

func TestRowAuthorKeepsDimOnSelection(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(0) // termenv.TrueColor: render the colors the test compares
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	m := newNotesModel(t)
	m.width = 80
	it := item.Item{ID: "!1", Title: "fix it", Author: "alice", Fields: map[string]any{}}
	want := sDim.Background(cSelBg).Render("alice")
	if want == "alice" {
		t.Fatal("test needs a color profile: styles rendered plain")
	}
	if got := m.row(it, true, 7); !strings.Contains(got, want) {
		t.Errorf("selected row = %q, want dim author with selection background %q", got, want)
	}
}

func TestApplyThemeTextDimSelection(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(0) // termenv.TrueColor: render the colors the test compares
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prev)
		applyTheme(config.Theme{})
	})
	render := func() (text, dim, tab string) {
		return sText.Render("x"), sDim.Render("x"), sTabOn.Render("x")
	}
	defText, defDim, defTab := render()
	applyTheme(config.Theme{Colors: config.Colors{Text: "#010203", Dim: "#040506", Selection: "#070809"}})
	text, dim, tab := render()
	for _, c := range []struct{ name, got, def, want string }{
		{"text", text, defText, "1;2;3"},
		{"dim", dim, defDim, "4;5;6"},
		{"selection", tab, defTab, "7;8;9"},
	} {
		if c.got == c.def || !strings.Contains(c.got, c.want) {
			t.Errorf("%s override not applied: %q (default %q)", c.name, c.got, c.def)
		}
	}
	applyTheme(config.Theme{})
	if text, dim, tab = render(); text != defText || dim != defDim || tab != defTab {
		t.Errorf("clearing the overrides should restore the defaults, got %q %q %q", text, dim, tab)
	}
}

// dirtyProv returns items carrying terminal control sequences from both Quick and Fetch.
type dirtyProv struct{}

func (dirtyProv) dirty() []item.Item {
	return []item.Item{{ID: "1", Title: "a\x1b[2Jb\nc", Author: "x\x07", Fields: map[string]any{"f": "\x1b]52;c;aGk=\x07"}}}
}
func (p dirtyProv) Fetch(context.Context) ([]item.Item, error) { return p.dirty(), nil }
func (p dirtyProv) Quick(context.Context) ([]item.Item, error) { return p.dirty(), nil }

func TestFetchStripsControlSequences(t *testing.T) {
	m := newBackoffModel(t)
	m.providers["p"] = dirtyProv{}
	msgs := runCmd(m.fetch("p"))
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want quick and full", len(msgs))
	}
	for i, msg := range msgs {
		fm, ok := msg.(fetchedMsg)
		if !ok {
			t.Fatalf("message %d is %T", i, msg)
		}
		it := fm.items[0]
		if it.Title != "a[2Jb c" || it.Author != "x" || it.Fields["f"] != "]52;c;aGk=" {
			t.Errorf("message %d (partial=%v) reached the model unsanitized: %#v", i, fm.partial, it)
		}
	}
}

func TestCachedItemsAreSanitized(t *testing.T) {
	m := newBackoffModel(t)
	cache.Save("p", dirtyProv{}.dirty())
	m.data = map[string][]item.Item{}
	m.applyConfig(&config.Config{Provider: map[string]config.Provider{"p": {Type: "command", Command: "true"}},
		View: []config.View{{Name: "v", Provider: "p", Filter: "true"}},
	})
	got := m.data["p"]
	if len(got) != 1 || got[0].Title != "a[2Jb c" || got[0].Fields["f"] != "]52;c;aGk=" {
		t.Errorf("cached items reached the model unsanitized: %#v", got)
	}
}
