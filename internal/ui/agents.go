package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/session"
	"github.com/chriha/lanes/internal/ui/flowview"
)

// agentPollEvery is how often the open agent view reads what the transcripts gained.
const agentPollEvery = 500 * time.Millisecond

// agentView is the open agent view: the flow view of one Claude session and the tailer that feeds it.
type agentView struct {
	name   string // the tmux session
	back   mode   // the screen A was pressed on
	fv     flowview.Model
	tailer *session.Tailer // read by one poll at a time
	gen    int             // tells this view's polls from those of a view opened before
	hook   string          // Claude's hook state as last applied
	ended  bool            // Claude exited or the session finished: the next poll is the last
	frozen bool            // the end is applied: no more polls
	err    string          // the last read error said, so a failing read is said once
}

// agentPollMsg says it is time for the agent view's next read of the transcripts.
type agentPollMsg struct{ gen int }

// agentSnapMsg carries what a read of the transcripts gave.
type agentSnapMsg struct {
	gen   int
	snap  session.Snapshot
	err   error
	final bool // the read started after the end was known: it holds the final state
}

// poll reads what the transcripts gained since the last read, off the UI goroutine. Only one read is in flight:
// the next is scheduled when this one has landed.
func (a *agentView) poll() tea.Cmd {
	t, gen, final := a.tailer, a.gen, a.ended
	return func() tea.Msg {
		snap, err := t.Poll()
		return agentSnapMsg{gen: gen, snap: snap, err: err, final: final}
	}
}

// claudeEnded tells whether Claude is done in the session: the session finished, or Claude exited and its hook
// state is gone.
func claudeEnded(s session.Info) bool { return s.Dead || s.Claude == "" }

// flowTheme maps the app's theme onto the agent view's roles: opus in the accent purple, sonnet in the warn
// orange, haiku in the info blue.
func flowTheme() *flowview.Theme {
	return flowview.NewTheme(flowview.Palette{
		Text: cText, Dim: cDim, Opus: cAccent, Sonnet: cWarn, Haiku: cInfo, OK: cOK, Bad: cBad, Selection: cSelBg,
	})
}

// agentsOfItem opens the agent view on the item's newest session with a Claude transcript.
func (m *Model) agentsOfItem(it item.Item) tea.Cmd {
	for _, s := range m.sessionsOf(it) { // newest first
		if session.ClaudeTranscript(s.Name) != "" {
			return m.openAgents(s, modeNormal)
		}
	}
	m.say(lvWarn, "no Claude session for this item")
	return nil
}

// agentsOfSession opens the agent view on session s, from the sessions screen.
func (m *Model) agentsOfSession(s session.Info) tea.Cmd {
	if session.ClaudeTranscript(s.Name) == "" {
		m.say(lvWarn, "no Claude session in "+s.Name)
		return nil
	}
	return m.openAgents(s, modeSessions)
}

// openAgents opens the agent view on session s, which has a Claude transcript, and returns to back on close. It
// starts the animation tick and the first read.
func (m *Model) openAgents(s session.Info, back mode) tea.Cmd {
	m.agentGen++
	a := &agentView{
		name:   s.Name,
		back:   back,
		fv:     flowview.New(flowTheme()),
		tailer: session.NewTailer(session.ClaudeTranscript(s.Name)),
		gen:    m.agentGen,
		hook:   s.Claude,
		ended:  claudeEnded(s),
	}
	a.fv.SetSize(m.width, m.height)
	a.fv.SetSession(s.Name, s.Created, time.Now())
	a.fv.Apply(flowview.HookEvent{Hook: s.Claude})
	m.agents, m.mode, m.pendingG = a, modeAgents, false
	return tea.Batch(a.fv.Init(), a.poll())
}

func (m *Model) closeAgents() {
	if m.agents != nil {
		m.mode = m.agents.back
	}
	m.agents = nil
}

func (m *Model) keyAgents(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "q", "A", "ctrl+c":
		m.closeAgents()
	}
	return m, nil
}

// onAgentPoll starts the next read of the open view; a tick of a closed or frozen view starts none.
func (m *Model) onAgentPoll(msg agentPollMsg) tea.Cmd {
	a := m.agents
	if a == nil || msg.gen != a.gen || a.frozen {
		return nil
	}
	return a.poll()
}

// onAgentSnap hands a read to the view and schedules the next one. The first read that started after the session
// ended is the final state: the view freezes on it and reads no more.
func (m *Model) onAgentSnap(msg agentSnapMsg) tea.Cmd {
	a := m.agents
	if a == nil || msg.gen != a.gen || a.frozen {
		return nil
	}
	if msg.err != nil && msg.err.Error() != a.err {
		m.say(lvWarn, "agent view: "+msg.err.Error())
	}
	a.err = ""
	if msg.err != nil {
		a.err = msg.err.Error()
	}
	a.fv.Apply(flowview.SnapshotEvent{Snap: msg.snap})
	switch {
	case msg.final:
		a.fv.Apply(flowview.EndEvent{})
		a.frozen = true
		return nil
	case a.ended: // the end came while this read was on its way: what came after it is read once more
		return a.poll()
	}
	gen := a.gen
	return tea.Tick(agentPollEvery, func(time.Time) tea.Msg { return agentPollMsg{gen: gen} })
}

// syncAgents passes the open view's session state from the newest tmux poll: a new hook state, or the end of
// Claude or of the session, which the next read freezes on.
func (m *Model) syncAgents() {
	a := m.agents
	if a == nil || a.ended {
		return
	}
	for _, s := range m.sessions {
		if s.Name != a.name {
			continue
		}
		if claudeEnded(s) {
			a.ended = true
		} else if s.Claude != a.hook {
			a.hook = s.Claude
			a.fv.Apply(flowview.HookEvent{Hook: s.Claude})
		}
		return
	}
	a.ended = true // the session is gone
}
