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
	st     State
	layout Layout
	canvas *Canvas

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
	return Model{canvas: NewCanvas(0, 0, theme), layout: NewLayout(0, 0), id: lastID.Add(1)}
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
	}
	return m, nil
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

// Offset returns the diagram's scroll offset: the column and line of the diagram at the viewport's top left. Not
// implemented yet.
func (m Model) Offset() (x, y int) { return 0, 0 }

// View draws the frame: the whole diagram, the compact layout or a resize hint, by size.
func (m Model) View() string {
	c, l, s := m.canvas, m.layout, m.frame()
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
	nodes, paths := s.drawTree(c, l)
	s.drawPackets(c, m.packets, nodes, paths, toolPath(l))
	s.drawLog(c, l.Log)
	s.drawStatus(c, l)
	return c.String()
}
