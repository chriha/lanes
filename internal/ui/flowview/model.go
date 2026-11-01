package flowview

import (
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/session"
)

// lastID numbers the models, so each one steps only on its own ticks.
var lastID atomic.Int64

// Model is the agent view as an embeddable sub-model: the parent sets its size and state, forwards its messages
// to Update and shows View. Events change the state; its own tick steps the animation while the view is open.
type Model struct {
	st      State
	layout  Layout
	canvas  *Canvas
	diagram *Canvas // the scrolling part, drawn whole and copied into the viewport
	offX    int     // the diagram's column at the viewport's left edge, as scrolled: clamped when used
	offY    int     // the diagram's line at the viewport's top, as scrolled: clamped when used

	id     int64     // tells this model's ticks from other models'
	gen    int       // tells the current tick chain from one a Resume replaced
	paused bool      // no ticking: the view is hidden
	last   time.Time // time of the last tick, zero before the first of a chain

	primed                  bool                           // a state is set: snapshots are diffed against seen
	seen                    map[string]session.AgentStatus // each agent's status in the last snapshot
	packets                 []Packet
	due                     map[string]time.Duration // each streaming edge's time to its next packet
	turns, calls, tokens    int                      // the lead's counters as shown, easing toward the state's
	tokIn, tokOut, tokCache int                      // the lead's token split as shown, easing like the counters
	typed                   float64                  // runes of the lead's last text shown
	phaseClock              time.Duration            // time since the last phase step
}

// tickMsg is a model's animation tick.
type tickMsg struct {
	id  int64
	gen int
	t   time.Time
}

// New returns an empty view that renders with theme (nil renders without styles).
func New(theme *Theme) Model {
	return Model{canvas: NewCanvas(0, 0, theme), diagram: NewCanvas(0, 0, theme), layout: NewLayout(0, 0), id: lastID.Add(1)}
}

// SetSize sets the terminal size the view fills.
func (m *Model) SetSize(w, h int) {
	m.canvas.Resize(w, h)
	m.layout = NewLayout(w, h)
}

// Init starts the animation tick; call it once, when the view opens. Nil when paused or the session ended.
func (m Model) Init() tea.Cmd {
	if m.paused || m.st.Ended {
		return nil
	}
	return m.tick()
}

func (m Model) tick() tea.Cmd {
	id, gen := m.id, m.gen
	return tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg{id: id, gen: gen, t: t} })
}

// Update steps the animation on its own ticks and applies events; other messages are ignored. The tick is not
// re-armed while paused or once the session ended, so a hidden or frozen view costs nothing.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		if msg.id != m.id || msg.gen != m.gen || m.paused || m.st.Ended {
			return m, nil
		}
		dt := tickEvery
		if !m.last.IsZero() {
			dt = min(max(msg.t.Sub(m.last), 0), maxStep)
		}
		m.last = msg.t
		m.Step(dt)
		m.st.Now = msg.t // the wall clock, even when the step was capped
		return m, m.tick()
	case Event:
		m.Apply(msg)
	case tea.KeyMsg:
		m.scroll(msg.String())
	}
	return m, nil
}

// scroll moves the viewport over the diagram for a key: j/k a line, ctrl+d/ctrl+u half the viewport, g/G to the
// top/bottom, h/l a column; the arrows like hjkl. It stops at the diagram's edges; other keys do nothing.
func (m *Model) scroll(key string) {
	const far = 1 << 30
	half := max(m.layout.View.H/2, 1)
	dx, dy := 0, 0
	switch key {
	case "j", "down":
		dy = 1
	case "k", "up":
		dy = -1
	case "ctrl+d":
		dy = half
	case "ctrl+u":
		dy = -half
	case "g":
		dy = -far
	case "G":
		dy = far
	case "h", "left":
		dx = -1
	case "l", "right":
		dx = 1
	default:
		return
	}
	d := m.layout.Diagram(m.st.Snap.Agents)
	x, y := m.clamp(d, m.offX, m.offY)
	m.offX, m.offY = m.clamp(d, x+dx, y+dy)
}

// clamp keeps an offset within the diagram d: the viewport never runs past its edges.
func (m Model) clamp(d Diagram, x, y int) (int, int) {
	v := m.layout.View
	return min(max(x, 0), max(d.W-v.W, 0)), min(max(y, 0), max(d.H-v.H, 0))
}

// Pause stops the animation tick, for a hidden view.
func (m *Model) Pause() { m.paused = true }

// Resume restarts the animation tick of a paused view, replacing a tick still in flight; nil when the session
// ended.
func (m *Model) Resume() tea.Cmd {
	m.paused = false
	m.gen++
	m.last = time.Time{}
	if m.st.Ended {
		return nil
	}
	return m.tick()
}

// SetSession names the session the view shows and sets its start and the frame's clock. Unlike SetState it keeps
// the state otherwise, so the first snapshot after it logs the agents seen so far without sending packets.
func (m *Model) SetSession(title string, start, now time.Time) {
	m.st.Title, m.st.Start, m.st.Now = title, start, now
}

// SetState sets what the view shows, without animating toward it: later snapshots animate from here.
func (m *Model) SetState(st State) {
	m.st = st
	m.track(st.Snap.Agents)
	m.settle()
}

// Offset returns the diagram's scroll offset: the column and line of the diagram at the viewport's top left. It
// is kept across snapshots and resizes and clamped to the diagram as it is now.
func (m Model) Offset() (x, y int) {
	return m.clamp(m.layout.Diagram(m.st.Snap.Agents), m.offX, m.offY)
}

// View draws the frame. The header, legend, session column, log and status line are pinned; the diagram (lead,
// tool layer and every agent) is drawn whole on a canvas of its own and the scrolled-to part of it copied into the
// viewport, with the position shown above the log when it does not fit.
func (m Model) View() string {
	c, l, s := m.canvas, m.layout, m.frame()
	c.Clear()
	d := l.Diagram(s.Snap.Agents)
	dc := m.diagram
	dc.Resize(d.W, d.H)
	s.drawLead(dc, d.Lead)
	s.drawTools(dc, d.Tools)
	paths := s.drawTree(dc, d)
	s.drawPackets(dc, m.packets, d.Nodes, paths, toolPath(d))
	x, y := m.clamp(d, m.offX, m.offY)
	c.Blit(dc, x, y, l.View)
	s.drawHeader(c, l)
	s.drawSession(c, l.Session)
	drawPosition(c, l, d, x, y)
	s.drawLog(c, l.Log)
	s.drawStatus(c, l)
	return c.String()
}
