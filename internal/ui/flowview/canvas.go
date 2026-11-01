package flowview

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Cell is one terminal cell: a single-width rune and the role it is drawn in.
type Cell struct {
	R    rune
	Role Role
}

// Rect is a rectangle in cells; X, Y is the top-left corner.
type Rect struct{ X, Y, W, H int }

// Canvas is a fixed-size grid of cells. Every drawing call clips to the grid, so panels may draw partly off-canvas.
type Canvas struct {
	W, H  int
	cells []Cell
	theme *Theme
}

// NewCanvas returns a blank w×h canvas that renders with theme (nil renders without styles).
func NewCanvas(w, h int, theme *Theme) *Canvas {
	c := &Canvas{theme: theme}
	c.Resize(w, h)
	return c
}

// Resize changes the size and blanks the canvas; negative sizes count as 0.
func (c *Canvas) Resize(w, h int) {
	c.W, c.H = max(w, 0), max(h, 0)
	if n := c.W * c.H; cap(c.cells) >= n {
		c.cells = c.cells[:n]
	} else {
		c.cells = make([]Cell, n)
	}
	c.Clear()
}

// Clear blanks every cell.
func (c *Canvas) Clear() {
	for i := range c.cells {
		c.cells[i] = Cell{R: ' '}
	}
}

// At returns the cell at x, y, or a blank cell outside the canvas.
func (c *Canvas) At(x, y int) Cell {
	if !c.in(x, y) {
		return Cell{R: ' '}
	}
	return c.cells[y*c.W+x]
}

func (c *Canvas) in(x, y int) bool { return x >= 0 && y >= 0 && x < c.W && y < c.H }

// Set draws r at x, y. A rune that is not exactly one cell wide (wide, zero-width, control) is drawn as '?', so a
// row never shifts the cells after it.
func (c *Canvas) Set(x, y int, r rune, role Role) {
	if !c.in(x, y) {
		return
	}
	c.cells[y*c.W+x] = Cell{R: singleWidth(r), Role: role}
}

// singleWidth returns r when it takes one cell, else '?'. ASCII and the box-drawing, block and geometric-shape
// blocks the view uses skip the width lookup.
func singleWidth(r rune) rune {
	switch {
	case r >= 0x20 && r < 0x7f, r >= 0x2500 && r <= 0x25ff:
		return r
	case r < 0x20, r == 0x7f:
		return '?'
	}
	if ansi.StringWidth(string(r)) != 1 {
		return '?'
	}
	return r
}

// Text draws s from x, y to the right, one cell per rune, and returns the x after its last rune.
func (c *Canvas) Text(x, y int, s string, role Role) int {
	for _, r := range s {
		c.Set(x, y, r, role)
		x++
	}
	return x
}

// HLine draws r on row y from x0 to x1, both inclusive, in either order.
func (c *Canvas) HLine(x0, x1, y int, r rune, role Role) {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	for x := max(x0, 0); x <= min(x1, c.W-1); x++ {
		c.Set(x, y, r, role)
	}
}

// VLine draws r in column x from y0 to y1, both inclusive, in either order.
func (c *Canvas) VLine(x, y0, y1 int, r rune, role Role) {
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	for y := max(y0, 0); y <= min(y1, c.H-1); y++ {
		c.Set(x, y, r, role)
	}
}

// Box draws the outline of rect with the border's runes and leaves the inside untouched; a rect smaller than
// 2×2 draws nothing.
func (c *Canvas) Box(rect Rect, b lipgloss.Border, role Role) {
	if rect.W < 2 || rect.H < 2 {
		return
	}
	x0, y0, x1, y1 := rect.X, rect.Y, rect.X+rect.W-1, rect.Y+rect.H-1
	first := func(s string) rune {
		for _, r := range s {
			return r
		}
		return ' '
	}
	c.HLine(x0+1, x1-1, y0, first(b.Top), role)
	c.HLine(x0+1, x1-1, y1, first(b.Bottom), role)
	c.VLine(x0, y0+1, y1-1, first(b.Left), role)
	c.VLine(x1, y0+1, y1-1, first(b.Right), role)
	c.Set(x0, y0, first(b.TopLeft), role)
	c.Set(x1, y0, first(b.TopRight), role)
	c.Set(x0, y1, first(b.BottomLeft), role)
	c.Set(x1, y1, first(b.BottomRight), role)
}

// Blit copies the cells of src from sx, sy into dst. Not implemented yet.
func (c *Canvas) Blit(src *Canvas, sx, sy int, dst Rect) {}

// String renders the rows joined by newlines. Adjacent cells of one role form a run that is styled once with the
// theme's precomputed prefix and suffix; blank unstyled cells stay plain spaces.
func (c *Canvas) String() string {
	var b strings.Builder
	b.Grow(c.W*c.H*2 + c.H)
	for y := 0; y < c.H; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		row := c.cells[y*c.W : (y+1)*c.W]
		for i := 0; i < len(row); {
			role := row[i].Role
			j := i
			for j < len(row) && row[j].Role == role {
				j++
			}
			c.writeRun(&b, row[i:j], role)
			i = j
		}
	}
	return b.String()
}

func (c *Canvas) writeRun(b *strings.Builder, run []Cell, role Role) {
	styled := c.theme != nil && role != RoleNone
	if styled {
		b.WriteString(c.theme.pre[role])
	}
	for _, cell := range run {
		b.WriteRune(cell.R)
	}
	if styled {
		b.WriteString(c.theme.post[role])
	}
}
