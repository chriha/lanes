// Package scrollbar computes the one-column scrollbar every scrollable area draws while its content overflows.
package scrollbar

const (
	// Thumb is the glyph of the part of the bar that stands for the visible window.
	Thumb = '█'
	// Track is the glyph of the rest of the bar.
	Track = '░'
)

// Rows returns one glyph per row of a bar length rows high, Thumb or Track, for an area that shows visible of total
// lines from offset on. It returns nil when there is nothing to scroll (total <= visible) or no room for a bar. The
// thumb is proportional to visible/total and at least one row, at the top at offset 0 and at the bottom at the
// last offset (total-visible); an offset outside that range counts as the nearest end.
func Rows(offset, visible, total, length int) []rune {
	if length < 1 || visible < 1 || total <= visible {
		return nil
	}
	scroll := total - visible
	offset = min(max(offset, 0), scroll)
	size := min(max((length*visible+total/2)/total, 1), length)
	start := ((length-size)*offset + scroll/2) / scroll
	out := make([]rune, length)
	for i := range out {
		if i >= start && i < start+size {
			out[i] = Thumb
		} else {
			out[i] = Track
		}
	}
	return out
}
