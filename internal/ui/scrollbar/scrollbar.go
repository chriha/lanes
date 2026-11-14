// Package scrollbar computes the one-column scrollbar every scrollable area draws while its content overflows.
package scrollbar

const (
	// Thumb is the glyph of the part of the bar that stands for the visible window.
	Thumb = '█'
	// Track is the glyph of the rest of the bar.
	Track = '░'
)

// Rows returns one glyph per row of a bar length rows high; not implemented yet: no bar.
func Rows(offset, visible, total, length int) []rune { return nil }
