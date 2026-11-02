package flowview

import (
	"math"
	"strings"
)

// segmentCount is the number of segments of a bar.
const segmentCount = 10

// Segments draws a bar of segmentCount one-cell glyphs at x, y, filled to v (clamped to 0..1) in 10% steps: the
// first n as glyph in fill, the rest as glyph in track. n is v rounded to the nearest tenth, but at least 1 for any
// v above 0. With blink the last filled segment is drawn in track, so a caller can make the bar's edge flash.
func (c *Canvas) Segments(x, y int, glyph rune, v float64, blink bool, fill, track Role) {
	v = min(max(v, 0), 1)
	n := int(math.Round(v * segmentCount))
	if v > 0 {
		n = max(n, 1)
	}
	for i := 0; i < segmentCount; i++ {
		r := track
		if i < n && !(blink && i == n-1) {
			r = fill
		}
		c.Set(x+i, y, glyph, r)
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
