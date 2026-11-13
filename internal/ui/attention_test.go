package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/session"
)

// the attention count is the number after the item count in a view's tab
func TestHeaderAttentionCount(t *testing.T) {
	b := func(kinds ...string) []item.Badge {
		var out []item.Badge
		for _, k := range kinds {
			out = append(out, item.Badge{Text: "x", Kind: k})
		}
		return out
	}
	sess := map[string][]session.Info{
		"a": {{Claude: "asking"}},
		"b": {{Claude: "working"}},
		"c": {{Dead: true, Exit: 1}},
		"d": {{Dead: true}},
		"e": {{Claude: "waiting"}, {Claude: "asking"}},
		"f": {{Dead: true, Claude: "waiting"}},
	}
	tests := []struct {
		name  string
		items []item.Item
		want  string // tab text after the name
	}{
		{"none", nil, "0"},
		{"warn badge alone", []item.Item{{ID: "z", Badges: b("warn", "bad")}}, "1"},
		{"asking session", []item.Item{{ID: "a"}}, "1 1"},
		{"working session", []item.Item{{ID: "b"}}, "1"},
		{"failed session", []item.Item{{ID: "c"}}, "1"},
		{"ok session", []item.Item{{ID: "d"}}, "1"},
		{"dead session", []item.Item{{ID: "f"}}, "1"},
		{"counted once", []item.Item{{ID: "e"}}, "1 1"},
		{"several items", []item.Item{{ID: "a"}, {ID: "b"}, {ID: "e"}}, "3 2"},
	}
	for _, tt := range tests {
		for _, active := range []int{0, 1} { // the count shows on the active tab too
			m := &Model{
				width:      200,
				cur:        active,
				sessByItem: sess,
				views:      []*view{{cfg: config.View{Name: "v"}, items: tt.items}, {cfg: config.View{Name: "w"}}},
				fetchedAt:  map[string]time.Time{},
			}
			got := strings.Join(strings.Fields(ansi.Strip(m.header())), " ")
			if !strings.Contains(got, " v "+tt.want+" w 0 ") {
				t.Errorf("%s (active %d): header %q lacks tab %q", tt.name, active, got, "v "+tt.want)
			}
		}
	}
}

func TestHeaderDivider(t *testing.T) {
	tests := []struct {
		name  string
		views []config.View
		want  string
	}{
		{"none", []config.View{{Name: "a"}, {Name: "b"}}, "a 0 b 0"},
		// {"before second", []config.View{{Name: "a"}, {Name: "b", Divider: true}, {Name: "c"}}, "a 0 · b 0 c 0"},
		{"before second", []config.View{{Name: "a"}, {Name: "b", Divider: true}, {Name: "c"}}, "a 0 ¦ b 0 c 0"},
		{"not before the first", []config.View{{Name: "a", Divider: true}, {Name: "b"}}, "a 0 b 0"},
	}
	for _, tt := range tests {
		m := &Model{width: 200, fetchedAt: map[string]time.Time{}}
		for _, v := range tt.views {
			m.views = append(m.views, &view{cfg: v})
		}
		got := strings.Join(strings.Fields(ansi.Strip(m.header())), " ")
		if !strings.Contains(got, tt.want) {
			t.Errorf("%s: header %q lacks %q", tt.name, got, tt.want)
		}
	}
}

// the built-in queue tab is set apart by one divider, unless it is the only tab
func TestHeaderDividerBeforeQueue(t *testing.T) {
	tests := []struct {
		name  string
		views []config.View
		want  string
		count int
	}{
		{"after a view", []config.View{{Name: "a"}}, "a 0 ¦ queue", 1},
		{"after a view with a divider", []config.View{{Name: "a"}, {Name: "b", Divider: true}}, "b 0 ¦ queue", 2},
		{"only tab", nil, "lanes queue", 0},
	}
	for _, tt := range tests {
		m := &Model{width: 200, fetchedAt: map[string]time.Time{}}
		for _, v := range tt.views {
			m.views = append(m.views, &view{cfg: v})
		}
		m.views = append(m.views, newQueueView())
		got := strings.Join(strings.Fields(ansi.Strip(m.header())), " ")
		if !strings.Contains(got, tt.want) {
			t.Errorf("%s: header %q lacks %q", tt.name, got, tt.want)
		}
		if n := strings.Count(got, "¦"); n != tt.count {
			t.Errorf("%s: header %q has %d dividers, want %d", tt.name, got, n, tt.count)
		}
	}
}

// dividers are not views: keys and numbering skip them
func TestDividerDoesNotChangeViewSwitching(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := New(&config.Config{
		Provider: map[string]config.Provider{"p": {Type: "command", Command: "true"}},
		View: []config.View{
			{Name: "a", Provider: "p", Filter: "true"},
			{Name: "b", Provider: "p", Filter: "true", Divider: true},
			{Name: "c", Provider: "p", Filter: "true"},
		},
	})
	m.width, m.height = 160, 20
	for _, tt := range []struct {
		keys []string
		want int
	}{{[]string{"l"}, 1}, {[]string{"l", "l"}, 2}, {[]string{"3"}, 2}, {[]string{"2"}, 1}, {[]string{"3", "h"}, 1}, {[]string{"Q"}, 3}, {[]string{"Q", "h"}, 2},
		{[]string{"S"}, 4}, {[]string{"Q", "l"}, 4}, {[]string{"S", "h"}, 3}, {[]string{"4"}, 0}, {[]string{"5"}, 0}} {
		m.cur = 0
		nav(m, tt.keys...)
		if m.cur != tt.want {
			t.Errorf("keys %v: cur = %d, want %d", tt.keys, m.cur, tt.want)
		}
	}
}
