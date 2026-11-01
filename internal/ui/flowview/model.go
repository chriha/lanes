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

	primed               bool                           // a state is set: snapshots are diffed against seen
	seen                 map[string]session.AgentStatus // each agent's status in the last snapshot
	packets              []Packet
	turns, calls, tokens int           // the lead's counters as shown, easing toward the state's
	typed                float64       // runes of the lead's last text shown
	phaseClock           time.Duration // time since the last phase step
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

// Init starts the animation tick; call it once, when the view opens.
func (m Model) Init() tea.Cmd { return nil }

// Update steps the animation on its own ticks and applies events; other messages are ignored.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) { return m, nil }

// Pause stops the animation tick, for a hidden view.
func (m *Model) Pause() {}

// Resume restarts the animation tick of a paused view; nil when the session ended.
func (m *Model) Resume() tea.Cmd { return nil }

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
