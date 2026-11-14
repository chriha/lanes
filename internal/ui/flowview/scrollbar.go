package flowview

import "github.com/chriha/lanes/internal/ui/scrollbar"

// VBar draws a vertical scrollbar of length rows from x, y down for an area showing visible of total lines from
// offset: the thumb in thumb, the track in RoleTrack. Nothing is drawn when the content fits.
func (c *Canvas) VBar(x, y, length, offset, visible, total int, thumb Role) {
	for i, r := range scrollbar.Rows(offset, visible, total, length) {
		c.Set(x, y+i, r, barRole(r, thumb))
	}
}

// HBar draws a horizontal scrollbar of length columns from x, y to the right for an area showing visible of total
// columns from offset, like VBar.
func (c *Canvas) HBar(x, y, length, offset, visible, total int, thumb Role) {
	for i, r := range scrollbar.Rows(offset, visible, total, length) {
		c.Set(x+i, y, r, barRole(r, thumb))
	}
}

func barRole(r rune, thumb Role) Role {
	if r == scrollbar.Thumb {
		return thumb
	}
	return RoleTrack
}

// barThumb is the role of a scrollbar's thumb: the accent while its area has the focus, else muted.
func barThumb(focused bool) Role {
	if focused {
		return RoleAccent
	}
	return RoleDim
}
