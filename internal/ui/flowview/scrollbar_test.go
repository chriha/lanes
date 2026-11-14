package flowview

import (
	"strings"
	"testing"

	"github.com/chriha/lanes/internal/ui/scrollbar"
)

func TestCanvasBars(t *testing.T) {
	c := NewCanvas(6, 6, nil)
	c.VBar(5, 1, 4, 10, 10, 20, RoleAccent) // at the bottom of 20 lines
	c.HBar(0, 5, 4, 0, 2, 8, RoleDim)       // at the left of 8 columns
	c.VBar(0, 0, 4, 0, 10, 10, RoleAccent)  // fits: nothing
	got := frameOf(c)
	if col := strings.Join(columnOf(got, 5), ""); col != " ░░██ " {
		t.Errorf("vertical bar column = %q, want the thumb at the bottom", col)
	}
	if row := string([]rune(got[5])[:4]); row != "█░░░" {
		t.Errorf("horizontal bar row = %q, want the thumb at the left", row)
	}
	if got[0][0] != ' ' {
		t.Errorf("a bar was drawn for content that fits")
	}
	if r := c.At(5, 4); r.R != scrollbar.Thumb || r.Role != RoleAccent {
		t.Errorf("thumb cell = %+v, want the thumb glyph in the accent", r)
	}
	if r := c.At(5, 1); r.R != scrollbar.Track || r.Role != RoleTrack {
		t.Errorf("track cell = %+v, want the track glyph in the track role", r)
	}
}

func frameOf(c *Canvas) []string { return strings.Split(c.String(), "\n") }

func columnOf(rows []string, x int) []string {
	var out []string
	for _, r := range rows {
		out = append(out, string([]rune(r)[x]))
	}
	return out
}

// barCells are the glyph runes of the frame in column x from row y0 to y1 (exclusive), ' ' where there is none.
func barCells(m Model, x, y0, y1 int) string {
	rows := frameRows(m)
	var b strings.Builder
	for y := y0; y < y1; y++ {
		r := rows[y][x]
		if r == scrollbar.Thumb || r == scrollbar.Track {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return b.String()
}

func TestAgentViewScrollbars(t *testing.T) {
	t.Run("diagram: both bars while it overflows, following the offsets", func(t *testing.T) {
		m := scrolled(withAgentsOf(deep()), 90, 24)
		f := m.layout.View
		v, d, vbar, hbar := m.viewport()
		if !vbar || !hbar || v.W != f.W-1 || v.H != f.H-1 {
			t.Fatalf("viewport %+v (vbar %v, hbar %v) in %+v, want both bars and one column and row less", v, vbar, hbar, f)
		}
		if got := barCells(m, f.X+f.W-1, v.Y, v.Y+v.H); !strings.HasPrefix(got, string(scrollbar.Thumb)) || !strings.HasSuffix(got, string(scrollbar.Track)) {
			t.Errorf("vertical bar at the top = %q, want the thumb first", got)
		}
		m, _ = m.Update(keyMsg("G"))
		if got := barCells(m, f.X+f.W-1, v.Y, v.Y+v.H); !strings.HasPrefix(got, string(scrollbar.Track)) || !strings.HasSuffix(got, string(scrollbar.Thumb)) {
			t.Errorf("vertical bar at the bottom = %q, want the thumb last", got)
		}
		row := string(frameRows(m)[f.Y+f.H-1][v.X : v.X+v.W])
		if !strings.HasPrefix(row, string(scrollbar.Thumb)) || !strings.HasSuffix(row, string(scrollbar.Track)) {
			t.Errorf("horizontal bar at the left = %q, want the thumb first", row)
		}
		m, _ = m.Update(keyMsg("l"))
		m.offX = far
		row = string(frameRows(m)[f.Y+f.H-1][v.X : v.X+v.W])
		if !strings.HasSuffix(row, string(scrollbar.Thumb)) {
			t.Errorf("horizontal bar at the right end = %q, want the thumb last", row)
		}
		if d.W <= v.W || d.H <= v.H {
			t.Errorf("diagram %dx%d does not overflow the viewport %+v", d.W, d.H, v)
		}
	})
	t.Run("nothing to scroll: no bars, the full viewport", func(t *testing.T) {
		m := scrolled(withAgentsOf(fixtureState().Snap.Agents), 200, 50)
		v, _, vbar, hbar := m.viewport()
		if vbar || hbar || v != m.layout.View {
			t.Errorf("viewport %+v (vbar %v, hbar %v), want the layout's %+v without bars", v, vbar, hbar, m.layout.View)
		}
		if strings.ContainsAny(string(frameRows(m)[m.layout.View.Y+m.layout.View.H-1]), "█░") {
			t.Error("a bar below a diagram that fits")
		}
	})
	t.Run("log: the bar follows the log's offset, none when it fits", func(t *testing.T) {
		st := fixtureState()
		st.Log = numberedLog(10)
		m := scrolled(st, 90, 24)
		l := m.layout.Log
		x := l.X + l.W - 2
		if got := barCells(m, x, l.Y+1, l.Y+l.H-1); got != "░░██" {
			t.Errorf("log bar following the newest = %q, want the thumb at the bottom", got)
		}
		m, _ = m.Update(keyMsg("tab"))
		m, _ = m.Update(keyMsg("g"))
		if got := barCells(m, x, l.Y+1, l.Y+l.H-1); got != "██░░" {
			t.Errorf("log bar at the oldest = %q, want the thumb at the top", got)
		}
		st.Log = numberedLog(3)
		if got := barCells(scrolled(st, 90, 24), x, l.Y+1, l.Y+l.H-1); strings.TrimSpace(got) != "" {
			t.Errorf("log bar for a log that fits = %q, want none", got)
		}
	})
	t.Run("session column: the bar follows its offset, none when it fits", func(t *testing.T) {
		m := scrolled(fixtureState(), 90, 24)
		r := m.layout.Session
		x := r.X + r.W - 2
		if got := barCells(m, x, r.Y+1, r.Y+r.H-1); !strings.HasPrefix(got, string(scrollbar.Thumb)) {
			t.Errorf("session bar at the top = %q, want the thumb first", got)
		}
		m, _ = m.Update(keyMsg("tab"))
		m, _ = m.Update(keyMsg("tab"))
		m, _ = m.Update(keyMsg("G"))
		if got := barCells(m, x, r.Y+1, r.Y+r.H-1); !strings.HasSuffix(got, string(scrollbar.Thumb)) {
			t.Errorf("session bar at the bottom = %q, want the thumb last", got)
		}
		m = scrolled(fixtureState(), 130, 60)
		r = m.layout.Session
		if got := barCells(m, r.X+r.W-2, r.Y+1, r.Y+r.H-1); strings.TrimSpace(got) != "" {
			t.Errorf("session bar for a column that fits = %q, want none", got)
		}
	})
}
