package flowview

// Model is the agent view as an embeddable sub-model: the parent sets its size and state and shows View.
type Model struct {
	st     State
	layout Layout
	canvas *Canvas
}

// New returns an empty view that renders with theme (nil renders without styles).
func New(theme *Theme) Model {
	return Model{canvas: NewCanvas(0, 0, theme), layout: NewLayout(0, 0)}
}

// SetSize sets the terminal size the view fills.
func (m *Model) SetSize(w, h int) {
	m.canvas.Resize(w, h)
	m.layout = NewLayout(w, h)
}

// SetState sets what the view shows.
func (m *Model) SetState(st State) { m.st = st }

// View draws the frame: the whole diagram, the compact layout or a resize hint, by size.
func (m Model) View() string {
	c, l, s := m.canvas, m.layout, m.st
	c.Clear()
	if l.Mode == ModeTooSmall {
		hint := "resize to ≥ 60×24"
		c.Text((c.W-len([]rune(hint)))/2, c.H/2, hint, RoleDim)
		return c.String()
	}
	s.drawHeader(c, l)
	if l.Mode == ModeFull {
		s.drawSession(c, l.Session)
		s.drawTools(c, l.Tools)
	}
	s.drawLead(c, l.Lead)
	s.drawTree(c, l)
	s.drawLog(c, l.Log)
	s.drawStatus(c, l)
	return c.String()
}
