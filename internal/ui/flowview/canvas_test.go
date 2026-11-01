package flowview

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// rows renders c without styles and splits it into rows.
func rows(c *Canvas) []string { return strings.Split(c.String(), "\n") }

func testPalette() Palette {
	ac := func(light, dark string) lipgloss.AdaptiveColor {
		return lipgloss.AdaptiveColor{Light: light, Dark: dark}
	}
	return Palette{
		Text:      ac("#1F2430", "#E6E8EE"),
		Dim:       ac("#8A90A0", "#6B7280"),
		Opus:      ac("#6C3FD1", "#A78BFA"),
		Sonnet:    ac("#B7791F", "#FBBF24"),
		Haiku:     ac("#2B6CB0", "#60A5FA"),
		OK:        ac("#1E8E4E", "#4ADE80"),
		Bad:       ac("#C53030", "#F87171"),
		Selection: ac("#DDD6F5", "#3A3560"),
	}
}

// trueColor renders styles with colors for the rest of the test.
func trueColor(t *testing.T) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(0) // termenv.TrueColor: render the colors the test compares
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

func TestCanvasSetClips(t *testing.T) {
	tests := []struct {
		name string
		x, y int
		want []string
	}{
		{"inside", 1, 1, []string{"   ", " x "}},
		{"top-left corner", 0, 0, []string{"x  ", "   "}},
		{"bottom-right corner", 2, 1, []string{"   ", "  x"}},
		{"left of the canvas", -1, 0, []string{"   ", "   "}},
		{"above the canvas", 0, -1, []string{"   ", "   "}},
		{"right of the canvas", 3, 0, []string{"   ", "   "}},
		{"below the canvas", 0, 2, []string{"   ", "   "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(3, 2, nil)
			c.Set(tt.x, tt.y, 'x', RoleText)
			if got := rows(c); strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("rows = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCanvasSetKeepsSingleWidthRunes(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want rune
	}{
		{"ascii", 'a', 'a'},
		{"box drawing", '┼', '┼'},
		{"double rule", '═', '═'},
		{"block shade", '▓', '▓'},
		{"packet", '●', '●'},
		{"arrow head", '▶', '▶'},
		{"spinner", '◐', '◐'},
		{"check mark", '✓', '✓'},
		{"middle dot", '·', '·'},
		{"wide CJK", '世', '?'},
		{"emoji", '🚀', '?'},
		{"tab", '\t', '?'},
		{"newline", '\n', '?'},
		{"combining accent", '́', '?'},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(3, 1, nil)
			c.Set(1, 0, tt.r, RoleText)
			if got := c.At(1, 0).R; got != tt.want {
				t.Errorf("rune = %q, want %q", got, tt.want)
			}
			if got := c.String(); len([]rune(got)) != 3 {
				t.Errorf("row %q is not 3 runes wide", got)
			}
		})
	}
}

func TestCanvasText(t *testing.T) {
	tests := []struct {
		name    string
		x       int
		s       string
		want    string
		wantEnd int
	}{
		{"fits", 1, "ab", " ab  ", 3},
		{"clipped right", 3, "abcd", "   ab", 7},
		{"clipped left", -2, "abcd", "cd   ", 2},
		{"runes, not bytes", 0, "◆ ok", "◆ ok ", 4},
		{"empty", 2, "", "     ", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(5, 1, nil)
			end := c.Text(tt.x, 0, tt.s, RoleText)
			if got := c.String(); got != tt.want {
				t.Errorf("row = %q, want %q", got, tt.want)
			}
			if end != tt.wantEnd {
				t.Errorf("end = %d, want %d", end, tt.wantEnd)
			}
		})
	}
}

func TestCanvasHLine(t *testing.T) {
	tests := []struct {
		name      string
		x0, x1, y int
		want      string
	}{
		{"left to right", 1, 3, 0, " ─── "},
		{"right to left", 3, 1, 0, " ─── "},
		{"single cell", 2, 2, 0, "  ─  "},
		{"clipped both ends", -3, 9, 0, "─────"},
		{"off the canvas", 6, 9, 0, "     "},
		{"row outside", 0, 4, 5, "     "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(5, 1, nil)
			c.HLine(tt.x0, tt.x1, tt.y, '─', RoleDim)
			if got := c.String(); got != tt.want {
				t.Errorf("row = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCanvasVLine(t *testing.T) {
	tests := []struct {
		name      string
		x, y0, y1 int
		want      []string
	}{
		{"top to bottom", 1, 0, 1, []string{" │ ", " │ ", "   "}},
		{"bottom to top", 1, 1, 0, []string{" │ ", " │ ", "   "}},
		{"clipped both ends", 0, -4, 9, []string{"│  ", "│  ", "│  "}},
		{"column outside", 3, 0, 2, []string{"   ", "   ", "   "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(3, 3, nil)
			c.VLine(tt.x, tt.y0, tt.y1, '│', RoleDim)
			if got := rows(c); strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("rows = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCanvasBox(t *testing.T) {
	tests := []struct {
		name string
		r    Rect
		b    lipgloss.Border
		want []string
	}{
		{"normal border", Rect{1, 0, 4, 3}, lipgloss.NormalBorder(), []string{" ┌──┐ ", " │  │ ", " └──┘ "}},
		{"rounded border", Rect{0, 0, 3, 2}, lipgloss.RoundedBorder(), []string{"╭─╮   ", "╰─╯   ", "      "}},
		{"smallest box", Rect{0, 1, 2, 2}, lipgloss.NormalBorder(), []string{"      ", "┌┐    ", "└┘    "}},
		{"clipped right and bottom", Rect{4, 1, 4, 4}, lipgloss.NormalBorder(), []string{"      ", "    ┌─", "    │ "}},
		{"clipped left and top", Rect{-2, -1, 4, 3}, lipgloss.NormalBorder(), []string{" │    ", "─┘    ", "      "}},
		{"too narrow", Rect{0, 0, 1, 3}, lipgloss.NormalBorder(), []string{"      ", "      ", "      "}},
		{"too flat", Rect{0, 0, 4, 1}, lipgloss.NormalBorder(), []string{"      ", "      ", "      "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(6, 3, nil)
			c.Box(tt.r, tt.b, RoleText)
			if got := rows(c); strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("rows = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCanvasBoxKeepsInside(t *testing.T) {
	c := NewCanvas(4, 3, nil)
	c.Set(1, 1, 'x', RoleText)
	c.Box(Rect{0, 0, 4, 3}, lipgloss.NormalBorder(), RoleDim)
	if got := c.At(1, 1); got != (Cell{R: 'x', Role: RoleText}) {
		t.Errorf("inside cell = %+v, want x in RoleText", got)
	}
}

func TestCanvasResizeAndClear(t *testing.T) {
	c := NewCanvas(2, 2, nil)
	c.Text(0, 0, "ab", RoleText)
	c.Clear()
	if got := c.String(); got != "  \n  " {
		t.Errorf("after Clear = %q, want blank", got)
	}
	c.Text(0, 0, "ab", RoleText)
	c.Resize(3, 1)
	if got := c.String(); got != "   " {
		t.Errorf("after Resize = %q, want a blank 3×1 row", got)
	}
	c.Resize(-1, 4)
	if c.W != 0 || c.H != 4 {
		t.Errorf("size = %d×%d, want 0×4", c.W, c.H)
	}
}

func TestCanvasStringStylesRuns(t *testing.T) {
	trueColor(t)
	theme := NewTheme(testPalette())
	c := NewCanvas(6, 2, theme)
	c.Text(0, 0, "ab", RoleOpus)
	c.Text(2, 0, "cd", RoleDim)
	c.Text(4, 0, "e", RoleOpus)
	c.Text(0, 1, "xy", RoleOpus)

	want := theme.pre[RoleOpus] + "ab" + theme.post[RoleOpus] +
		theme.pre[RoleDim] + "cd" + theme.post[RoleDim] +
		theme.pre[RoleOpus] + "e" + theme.post[RoleOpus] + " " +
		"\n" + theme.pre[RoleOpus] + "xy" + theme.post[RoleOpus] + "    "
	if got := c.String(); got != want {
		t.Errorf("String() = %q\nwant       %q", got, want)
	}
}

func TestCanvasStringWithoutThemeIsPlain(t *testing.T) {
	trueColor(t)
	c := NewCanvas(3, 1, nil)
	c.Text(0, 0, "abc", RoleOpus)
	if got := c.String(); got != "abc" {
		t.Errorf("String() = %q, want plain abc", got)
	}
}

func TestNewThemeRendersEachRole(t *testing.T) {
	trueColor(t)
	theme := NewTheme(testPalette())
	if theme.pre[RoleNone] != "" || theme.post[RoleNone] != "" {
		t.Errorf("RoleNone = %q…%q, want unstyled", theme.pre[RoleNone], theme.post[RoleNone])
	}
	seen := map[string]Role{}
	for r := RoleText; r < numRoles; r++ {
		if !strings.HasPrefix(theme.pre[r], "\x1b[") || theme.post[r] == "" {
			t.Errorf("role %d = %q…%q, want an ANSI prefix and a reset", r, theme.pre[r], theme.post[r])
		}
		if other, dup := seen[theme.pre[r]]; dup {
			t.Errorf("roles %d and %d render the same prefix %q", other, r, theme.pre[r])
		}
		seen[theme.pre[r]] = r
	}
}

func TestModelRole(t *testing.T) {
	tests := []struct {
		model string
		want  Role
	}{
		{"claude-opus-4-1-20250805", RoleOpus},
		{"claude-opus-4-6[1m]", RoleOpus},
		{"opus", RoleOpus},
		{"claude-sonnet-4-5", RoleSonnet},
		{"Sonnet", RoleSonnet},
		{"claude-haiku-4-5", RoleHaiku},
		{"claude-3-5-haiku-latest", RoleHaiku},
		{"gpt-5", RoleText},
		{"", RoleText},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := ModelRole(tt.model); got != tt.want {
				t.Errorf("ModelRole(%q) = %d, want %d", tt.model, got, tt.want)
			}
		})
	}
}

// sink keeps the benchmarked String() call from being optimised away.
var sink string

// BenchmarkCanvasString renders a full 90×46 frame whose roles change every few cells, a worst case for runs.
func BenchmarkCanvasString(b *testing.B) {
	c := NewCanvas(90, 46, NewTheme(testPalette()))
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; x++ {
			c.Set(x, y, '─', Role(1+(x/3+y)%int(numRoles-1)))
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = c.String()
	}
}

func TestBlit(t *testing.T) {
	src := NewCanvas(4, 3, nil)
	for y, row := range []string{"abcd", "efgh", "ijkl"} {
		src.Text(0, y, row, RoleText)
	}
	tests := []struct {
		name   string
		sx, sy int
		dst    Rect
		want   string
	}{
		{"the whole source", 0, 0, Rect{1, 1, 4, 3}, "......\n.abcd.\n.efgh.\n.ijkl."},
		{"a window of it", 1, 1, Rect{0, 0, 2, 2}, "fg....\njk....\n......\n......"},
		{"past the source's end is blank", 3, 2, Rect{0, 0, 3, 2}, "l  ...\n   ...\n......\n......"},
		{"clipped to the canvas", 0, 0, Rect{4, 2, 4, 3}, "......\n......\n....ab\n....ef"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(6, 4, nil)
			for y := 0; y < c.H; y++ {
				c.HLine(0, c.W-1, y, '.', RoleDim)
			}
			c.Blit(src, tt.sx, tt.sy, tt.dst)
			if got := c.String(); got != tt.want {
				t.Errorf("blit =\n%s\nwant\n%s", got, tt.want)
			}
			if tt.name == "the whole source" && c.At(1, 1).Role != RoleText {
				t.Errorf("blitted cell has role %d, want the source's", c.At(1, 1).Role)
			}
		})
	}
}
