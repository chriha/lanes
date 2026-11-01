package flowview

import "strings"

// shades are the bar's edge glyphs from light to dark.
var shades = [3]rune{'░', '▒', '▓'}

// Bar draws a w-cell bar filled to v (clamped to 0..1) at x, y: full cells as █ in fill, a partly filled edge cell
// as a shade in fill, the rest as a ░ track in track. The edge shade comes from the fill fraction of that cell,
// shifted by phase so it shimmers as phase advances; an empty or full bar has no edge cell.
func (c *Canvas) Bar(x, y, w int, v float64, phase int, fill, track Role) {
	if w <= 0 {
		return
	}
	v = min(max(v, 0), 1)
	filled := v * float64(w)
	full := int(filled)
	for i := 0; i < w; i++ {
		switch {
		case i < full:
			c.Set(x+i, y, '█', fill)
		case i == full && filled > float64(full):
			frac := filled - float64(full)
			idx := (int(frac*float64(len(shades))) + phase%len(shades) + len(shades)) % len(shades)
			c.Set(x+i, y, shades[idx], fill)
		default:
			c.Set(x+i, y, '░', track)
		}
	}
}

// effortMeter is the 4-cell meter of an effort level: low ▮▯▯▯, medium ▮▮▯▯, high ▮▮▮▯, xhigh and max ▮▮▮▮; ""
// for an unknown level.
func effortMeter(level string) string {
	n := map[string]int{"low": 1, "medium": 2, "high": 3, "xhigh": 4, "max": 4}[strings.ToLower(level)]
	if n == 0 {
		return ""
	}
	return strings.Repeat("▮", n) + strings.Repeat("▯", 4-n)
}
