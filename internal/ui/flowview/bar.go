package flowview

import (
	"math"
	"strings"
)

// segmentCount is the number of segments of a bar.
const segmentCount = 10

// halfGlyph is the half-filled cell of a bar.
const halfGlyph = '▌'

// Segments draws a bar of segmentCount one-cell glyphs at x, y, filled to v (clamped to 0..1) in 5% steps: the
// full cells as glyph in fill, then at most one half cell as ▌ in fill, the rest as glyph in track. v is rounded to
// the nearest 5% (half a cell), but to at least one half cell for any v above 0.
func (c *Canvas) Segments(x, y, width int, glyph rune, v float64, fill, track Role) {
	v = min(max(v, 0), 1)
	halves := int(math.Round(v * segmentCount * 2))
	if v > 0 {
		halves = max(halves, 1)
	}
	for i := 0; i < segmentCount; i++ {
		switch {
		case i < halves/2:
			c.Set(x+i, y, glyph, fill)
		case i == halves/2 && halves%2 == 1:
			c.Set(x+i, y, halfGlyph, fill)
		default:
			c.Set(x+i, y, glyph, track)
		}
	}
}

// effortMeter is the 4-cell meter of an effort level: low ▮▯▯▯, medium ▮▮▯▯, high ▮▮▮▯, xhigh and max ▮▮▮▮; ""
// for an unknown level.
func effortMeter(level string) string {
	var n int
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "low":
		n = 1
	case "medium":
		n = 2
	case "high":
		n = 3
	case "xhigh", "max":
		n = 4
	default:
		return ""
	}
	return strings.Repeat("▮", n) + strings.Repeat("▯", 4-n)
}

// sparks are the sparkline's glyphs from the lowest to the highest level.
var sparks = [...]rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// Spark draws a sparkline of counts at x, y, one cell per count, scaled to the highest: a zero as ▁ in track, any
// other count as a glyph from ▂ to █ in fill.
func (c *Canvas) Spark(x, y int, counts []int, fill, track Role) {
	top := 0
	for _, n := range counts {
		top = max(top, n)
	}
	for i, n := range counts {
		if n <= 0 {
			c.Set(x+i, y, sparks[0], track)
			continue
		}
		c.Set(x+i, y, sparks[(n*(len(sparks)-1)+top-1)/top], fill)
	}
}
