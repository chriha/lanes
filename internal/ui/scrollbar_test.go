package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/ui/scrollbar"
)

// trimBar is a screen line stripped of its padding and its scrollbar glyph.
func trimBar(l string) string {
	l = strings.TrimSpace(l)
	l = strings.TrimRight(l, string(scrollbar.Thumb)+string(scrollbar.Track))
	return strings.TrimSpace(l)
}

// barOf is the scrollbar glyph column of the screen lines, one rune per line (a space where a line has none).
func barOf(lines []string) []rune {
	var glyphs []rune
	for _, l := range lines {
		r := []rune(ansi.Strip(l))
		if n := len(r); n > 0 && (r[n-1] == scrollbar.Thumb || r[n-1] == scrollbar.Track) {
			glyphs = append(glyphs, r[n-1])
		} else {
			glyphs = append(glyphs, ' ')
		}
	}
	return glyphs
}

func manyItems(n int) []item.Item {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("i%02d", i)
	}
	return mkItems(ids...)
}

func listArea(m *Model) []string {
	return strings.Split(strings.TrimSuffix(m.list(), "\n"), "\n")
}

func TestListScrollbar(t *testing.T) {
	tests := []struct {
		name    string
		n       int
		keys    []string
		wantBar string // glyph per list row; "" = none
	}{
		{"fits: no bar", 4, nil, ""},
		{"exactly fits: no bar", 6, nil, ""},
		{"overflow at the top", 12, nil, "███░░░"},
		{"follows the cursor to the end", 12, []string{"G"}, "░░░███"},
		{"moves with the offset", 12, []string{"j", "j", "j", "j", "j", "j", "j"}, "░███░░"},
	}
	for _, tt := range tests {
		for _, w := range []int{40, 80} {
			t.Run(fmt.Sprintf("%s/w%d", tt.name, w), func(t *testing.T) {
				m := newBackoffModel(t)
				m.width, m.height = w, 10 // list height 6
				m.Update(fetchedMsg{name: "p", items: manyItems(tt.n)})
				nav(m, tt.keys...)
				lines := listArea(m)
				if got := strings.TrimSpace(string(barOf(lines))); got != tt.wantBar {
					t.Errorf("bar = %q, want %q:\n%s", got, tt.wantBar, strings.Join(lines, "\n"))
				}
				for _, l := range lines {
					if lw := lipgloss.Width(l); lw > w {
						t.Errorf("line is %d wide, screen %d: %q", lw, w, ansi.Strip(l))
					}
				}
				if tt.wantBar != "" {
					for _, l := range lines {
						if lw := lipgloss.Width(l); lw != w {
							t.Errorf("barred line is %d wide, want the screen's %d: %q", lw, w, ansi.Strip(l))
						}
					}
				}
				if m.width != w {
					t.Errorf("width = %d after rendering, want %d", m.width, w)
				}
			})
		}
	}
}

func TestSessionsScrollbar(t *testing.T) {
	var ns []string
	for i := 0; i < 40; i++ {
		ns = append(ns, fmt.Sprintf("s%02d", i))
	}
	tests := []struct {
		name  string
		n     int
		keys  []string
		first rune // the bar's first glyph, ' ' for none
		last  rune
	}{
		{"fits: no bar", 3, nil, ' ', ' '},
		{"overflow at the top", 40, nil, scrollbar.Thumb, scrollbar.Track},
		{"overflow at the bottom", 40, []string{"G"}, scrollbar.Track, scrollbar.Thumb},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := sessModel(t, names(ns[:tt.n]...)...)
			m.width, m.height = 40, 12
			press(m, append([]string{"S"}, tt.keys...)...)
			lines := strings.Split(strings.TrimSuffix(m.sessionsList(), "\n"), "\n")
			bar := barOf(lines)
			if bar[0] != tt.first || bar[len(bar)-1] != tt.last {
				t.Errorf("bar %q, want it from %q to %q", string(bar), tt.first, tt.last)
			}
			for _, l := range lines {
				if lw := lipgloss.Width(l); tt.first != ' ' && lw != 40 {
					t.Errorf("line is %d wide, want the screen's 40: %q", lw, ansi.Strip(l))
				}
			}
		})
	}
}

func TestHelpAndDetailScrollbar(t *testing.T) {
	for _, w := range []int{40, 80} {
		for _, mode := range []string{"help", "detail"} {
			t.Run(fmt.Sprintf("%s/w%d", mode, w), func(t *testing.T) {
				m := newBackoffModel(t)
				m.width, m.height = w, 12
				m.Update(fetchedMsg{name: "p", items: manyItems(3)})
				if mode == "help" {
					m.mode = modeHelp
				} else {
					m.detail = m.items()[0]
					m.detail.Fields = map[string]any{"ci_jobs": []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n"}}
					m.mode = modeDetail
				}
				screen := func() []string { return strings.Split(m.View(), "\n") }
				body := func() []string { s := screen(); return s[:len(s)-1] }
				bar := func() string { return string(barOf(body())) }
				if got := bar(); []rune(got)[0] != scrollbar.Thumb || []rune(got)[len([]rune(got))-1] != scrollbar.Track {
					t.Fatalf("at the top: bar %q, want the thumb first and the track last", got)
				}
				for _, l := range body() {
					if lw := lipgloss.Width(l); lw != w {
						t.Errorf("line is %d wide, want %d: %q", lw, w, ansi.Strip(l))
					}
				}
				if foot := ansi.Strip(screen()[len(screen())-1]); !strings.Contains(foot, " of ") {
					t.Errorf("footer lacks the text hint: %q", foot)
				}
				// j moves the thumb down, G puts it at the bottom, g back at the top
				for i := 0; i < 1000; i++ {
					nav(m, "j")
				}
				if got := bar(); []rune(got)[len([]rune(got))-1] != scrollbar.Thumb {
					t.Errorf("scrolled to the end: bar %q, want the thumb last", got)
				}
				nav(m, "g")
				if got := bar(); []rune(got)[0] != scrollbar.Thumb {
					t.Errorf("g: bar %q, want the thumb first", got)
				}
				nav(m, "G")
				if got := bar(); []rune(got)[len([]rune(got))-1] != scrollbar.Thumb {
					t.Errorf("G: bar %q, want the thumb last", got)
				}
				nav(m, "g")
				for i := 0; i < 5; i++ { // the help wraps long rows, so it takes a few pages to move the thumb a row
					nav(m, "ctrl+d")
				}
				if got := bar(); []rune(got)[0] != scrollbar.Track {
					t.Errorf("ctrl-d: bar %q, want the thumb moved down", got)
				}
			})
		}
	}
	t.Run("no bar when it fits", func(t *testing.T) {
		m := newBackoffModel(t)
		m.width, m.height = 80, 200
		m.Update(fetchedMsg{name: "p", items: manyItems(3)})
		m.detail = m.items()[0]
		m.mode = modeDetail
		for _, l := range strings.Split(m.View(), "\n") {
			if strings.ContainsAny(l, string([]rune{scrollbar.Thumb, scrollbar.Track})) {
				t.Errorf("bar on a screen that fits: %q", l)
			}
		}
	})
}

func TestHelpWrapsLongRowsWithScrollbar(t *testing.T) {
	const qRow = "queue view: starts waiting for a free session (r starts entries now, past the limit; x drops them)"
	m := newBackoffModel(t)
	m.width, m.height = 80, 20
	m.Update(fetchedMsg{name: "p", items: manyItems(3)})
	m.mode = modeHelp
	var seen []string
	for i := 0; i < 20; i++ {
		s := strings.Split(m.View(), "\n")
		if b := barOf(s[:len(s)-1]); b[0] == ' ' {
			t.Fatalf("no scrollbar on the help screen at page %d", i)
		}
		for _, l := range s[:len(s)-1] {
			seen = append(seen, trimBar(ansi.Strip(l)))
		}
		nav(m, "ctrl+d")
	}
	if got := strings.Join(strings.Fields(strings.Join(seen, " ")), " "); !strings.Contains(got, qRow) {
		t.Errorf("the Q row's full text is not on the barred help screen:\n%s", got)
	}
}
