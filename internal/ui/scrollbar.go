package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/ui/scrollbar"
)

// withScrollbar joins rows into a block of lines, each padded or cut to m.width cells, and appends the scrollbar
// column right of them when the area (rows shown, total lines, first shown line offset) overflows. Without an
// overflow the rows join unchanged. The caller builds the rows for a width one less than the screen's when it
// overflows.
func (m *Model) withScrollbar(rows []string, offset, total int) string {
	bar := scrollbar.Rows(offset, len(rows), total, len(rows))
	if bar == nil {
		return strings.Join(rows, "\n") + "\n"
	}
	var b strings.Builder
	for i, r := range rows {
		r = ansi.Truncate(r, m.width, "")
		b.WriteString(r + strings.Repeat(" ", max(m.width-ansi.StringWidth(r), 0)))
		if bar[i] == scrollbar.Thumb {
			b.WriteString(sBarThumb.Render(string(bar[i])))
		} else {
			b.WriteString(sBarTrack.Render(string(bar[i])))
		}
		b.WriteByte('\n')
	}
	return b.String()
}
