package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/session"
	"github.com/chriha/lanes/internal/ui/flowview"
)

// agentPollEvery is how often the open agent view reads what the transcripts gained; agentProcsEvery how often it
// samples the session's processes.
const (
	agentPollEvery  = 500 * time.Millisecond
	agentProcsEvery = 3 * time.Second
)

// agentView is the open agent view: the flow view of one Claude session and the tailer that feeds it.
type agentView struct {
	name   string // the tmux session
	back   mode   // the screen A was pressed on
	fv     flowview.Model
	tailer *session.Tailer              // read by one poll at a time
	gen    int                          // tells this view's polls from those of a view opened before
	hook   string                       // Claude's hook state as last applied
	ended  bool                         // Claude exited or the session finished: the next poll is the last
	frozen bool                         // the end is applied: no more polls
	err    string                       // the last read error said, so a failing read is said once
	has5h  bool                         // the view has a known 5h usage
	pid    int                          // the session's pane process, the root of the sampled tree; 0 when unknown
	sample func(pid int) []session.Proc // samples the process tree under pid; one sample in flight
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

// agentProcPollMsg says it is time for the agent view's next sample of the session's processes.
type agentProcPollMsg struct{ gen int }

// agentProcsMsg carries what a sample of the session's processes gave.
type agentProcsMsg struct {
	gen   int
	procs []session.Proc
}

// sampleProcs samples the session's process tree off the UI goroutine. Only one sample is in flight: the next is
// scheduled when this one has landed.
func (a *agentView) sampleProcs() tea.Cmd {
	sample, pid, gen := a.sample, a.pid, a.gen
	return func() tea.Msg { return agentProcsMsg{gen: gen, procs: sample(pid)} }
}

// sampling tells whether the view samples the session's processes: while Claude runs in a pane of known pid.
func (a *agentView) sampling() bool { return a.pid > 0 && !a.ended && !a.frozen }

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

// flowTheme renders the agent view's palette: the neon defaults with the [theme.agent_view] overrides.
func flowTheme() *flowview.Theme { return flowview.NewTheme(cAgent) }

// agentsOfItem opens the agent view on the item's newest session with a Claude transcript.
func (m *Model) agentsOfItem(it item.Item) tea.Cmd {
	for _, s := range m.sessionsOf(it) { // newest first
		if session.ClaudeTranscript(s.Name) != "" {
			return m.openAgents(s, modeNormal, it.Title)
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
	return m.openAgents(s, modeSessions, "")
}

// openAgents opens the agent view on session s, which has a Claude transcript, titled with the item's title ("" for
// the session name), and returns to back on close. It starts the animation tick and the first read.
func (m *Model) openAgents(s session.Info, back mode, title string) tea.Cmd {
	m.agentGen++
	a := &agentView{
		name:   s.Name,
		back:   back,
		fv:     flowview.New(flowTheme()),
		tailer: session.NewTailer(session.ClaudeTranscript(s.Name)),
		gen:    m.agentGen,
		hook:   s.Claude,
		ended:  claudeEnded(s),
		pid:    s.PID,
		sample: m.newSampler(),
	}
	a.fv.SetSize(m.width, m.height)
	a.fv.SetSession(s.Name, title, s.Created, time.Now())
	a.fv.Apply(flowview.HookEvent{Hook: s.Claude})
	m.agents, m.mode, m.pendingG = a, modeAgents, false
	m.syncUsage5h()
	cmds := []tea.Cmd{a.fv.Init(), a.poll()}
	if a.sampling() {
		cmds = append(cmds, a.sampleProcs())
	}
	return tea.Batch(cmds...)
}

// newSampler returns the process sampler of a new agent view: one per view, as a sampler follows one tree.
func (m *Model) newSampler() func(pid int) []session.Proc {
	if m.sampler != nil {
		return m.sampler()
	}
	return session.NewSampler().Sample
}

func (m *Model) closeAgents() {
	if m.agents != nil {
		m.mode = m.agents.back
	}
	m.agents = nil
}

// keyAgents closes the agent view on esc, q, A or ctrl+c and hands every other key to it: its scroll keys move the
// diagram.
func (m *Model) keyAgents(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "q", "A", "ctrl+c":
		m.closeAgents()
		return m, nil
	}
	var cmd tea.Cmd
	if m.agents != nil {
		m.agents.fv, cmd = m.agents.fv.Update(k)
	}
	return m, cmd
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

// onAgentProcPoll starts the next sample of the open view's processes; a tick of a closed or ended view starts none.
func (m *Model) onAgentProcPoll(msg agentProcPollMsg) tea.Cmd {
	a := m.agents
	if a == nil || msg.gen != a.gen || !a.sampling() {
		return nil
	}
	return a.sampleProcs()
}

// onAgentProcs hands a sample to the view and schedules the next one. A sample of a closed view, or one landing
// after Claude or the session ended, is dropped and ends the sampling.
func (m *Model) onAgentProcs(msg agentProcsMsg) tea.Cmd {
	a := m.agents
	if a == nil || msg.gen != a.gen || !a.sampling() {
		return nil
	}
	a.fv.Apply(flowview.ProcsEvent{Procs: msg.procs})
	gen := a.gen
	return tea.Tick(agentProcsEvery, func(time.Time) tea.Msg { return agentProcPollMsg{gen: gen} })
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

// syncUsage5h passes the account's 5h usage to the open view; a frozen view keeps the figure it ended with, once
// it has one.
func (m *Model) syncUsage5h() {
	if a := m.agents; a != nil && (!a.frozen || !a.has5h) {
		a.fv.Apply(flowview.UsageEvent{Pct: m.usage5h.pct, Known: m.usage5h.ok})
		a.has5h = m.usage5h.ok
	}
}
