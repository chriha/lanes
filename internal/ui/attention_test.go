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
	}
	tests := []struct {
		name  string
		items []item.Item
		want  string // tab text after the name
	}{
		{"none", nil, "0"},
		{"warn badge", []item.Item{{ID: "z", Badges: b("warn")}}, "1 1"},
		{"bad badge", []item.Item{{ID: "z", Badges: b("bad")}}, "1 1"},
		{"ok info dim never count", []item.Item{{ID: "z", Badges: b("ok", "info", "dim")}}, "1"},
		{"asking session", []item.Item{{ID: "a"}}, "1 1"},
		{"working session", []item.Item{{ID: "b"}}, "1"},
		{"failed session", []item.Item{{ID: "c"}}, "1 1"},
		{"ok session", []item.Item{{ID: "d"}}, "1"},
		{"counted once", []item.Item{{ID: "e", Badges: b("warn", "bad")}}, "1 1"},
		{"several items", []item.Item{{ID: "a"}, {ID: "b"}, {ID: "z", Badges: b("warn")}}, "3 2"},
	}
	for _, tt := range tests {
		for _, active := range []int{0, 1} { // the count shows on the active tab too
			m := &Model{
				width:      200,
				cur:        active,
				sessByItem: sess,
				views:      []*view{{cfg: config.View{Name: "v"}, items: tt.items}, {cfg: config.View{Name: "w"}}},
				fetchedAt:  map[string]time.Time{},
				loading:    map[string]bool{},
			}
			got := strings.Join(strings.Fields(ansi.Strip(m.header())), " ")
			if !strings.Contains(got, " v "+tt.want+" w 0 ") {
				t.Errorf("%s (active %d): header %q lacks tab %q", tt.name, active, got, "v "+tt.want)
			}
		}
	}
}
