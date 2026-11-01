package flowview

import (
	"sort"
	"time"

	"github.com/chriha/lanes/internal/session"
)

const (
	tickEvery  = 70 * time.Millisecond  // the animation tick
	maxStep    = 250 * time.Millisecond // the longest step one tick takes, after a stall or a pause
	phaseEvery = 140 * time.Millisecond // one spinner frame and shimmer step
	packetTime = 800 * time.Millisecond // a packet's trip along its connector
	easeTime   = 400 * time.Millisecond // a counter covers a step's share dt/easeTime of the gap to its target
	typeRate   = 40.0                   // runes per second the typewriter types
	maxLog     = 100                    // session log lines kept
)

// Packet is a dot travelling along an agent's connector: down from the parent when the agent spawns, up to the
// parent when it finishes.
type Packet struct {
	Agent string  // the agent whose connector it travels
	Up    bool    // child to parent (finish) instead of parent to child (spawn)
	Pos   float64 // progress along the connector, 0..1
}

// Step advances the animation by dt: the clock, packets, counters, the typewriter, the spinner and the shimmer.
// It does no I/O. An ended session doesn't move.
func (m *Model) Step(dt time.Duration) {
	if dt <= 0 || m.st.Ended {
		return
	}
	m.st.Now = m.st.Now.Add(dt)
	m.phaseClock += dt
	for m.phaseClock >= phaseEvery {
		m.st.Phase++
		m.phaseClock -= phaseEvery
	}
	var kept []Packet
	for _, p := range m.packets {
		if p.Pos += float64(dt) / float64(packetTime); p.Pos < 1 {
			kept = append(kept, p)
		}
	}
	m.packets = kept
	lead := m.st.Snap.Lead
	m.turns = ease(m.turns, lead.Turns, dt)
	m.calls = ease(m.calls, lead.Calls, dt)
	m.tokens = ease(m.tokens, lead.Tokens, dt)
	m.typed = min(m.typed+typeRate*dt.Seconds(), float64(len([]rune(lead.LastText))))
}

// ease moves cur toward target by the share dt/easeTime of the gap, at least 1, never past target.
func ease(cur, target int, dt time.Duration) int {
	gap := target - cur
	step := int(float64(gap) * float64(dt) / float64(easeTime))
	switch {
	case gap == 0:
		return cur
	case step == 0 && gap > 0:
		step = 1
	case step == 0:
		step = -1
	}
	if gap > 0 && step > gap || gap < 0 && step < gap {
		step = gap
	}
	return cur + step
}

// Apply takes an event into the view's state: a snapshot sends packets and log lines for agents that spawned or
// finished since the last one, a hook event moves the checkpoint, the end event freezes the view.
func (m *Model) Apply(ev Event) {
	switch ev := ev.(type) {
	case SnapshotEvent:
		m.applySnapshot(ev.Snap)
	case HookEvent:
		m.st.Hook = ev.Hook
	case EndEvent:
		m.st.Ended = true
	}
	if m.st.Ended {
		m.settle()
	}
}

func (m *Model) applySnapshot(snap session.Snapshot) {
	if snap.Lead.LastText != m.st.Snap.Lead.LastText {
		m.typed = 0
	}
	m.st.Snap = snap
	byID := make(map[string]session.Agent, len(snap.Agents))
	for _, a := range snap.Agents {
		byID[a.ID] = a
	}
	spawner := func(a session.Agent) (string, string) {
		if p, ok := byID[a.Parent]; ok {
			return p.Name, p.Model
		}
		return "lead", snap.Lead.Model
	}
	var lines []LogLine
	for _, a := range snap.Agents {
		prev, seen := m.seen[a.ID]
		if !seen {
			actor, model := spawner(a)
			lines = append(lines, LogLine{At: m.at(a.Start), Actor: actor, Model: model, Text: "spawned " + a.Name})
			if m.primed && a.Status == session.AgentRunning {
				m.packets = append(m.packets, Packet{Agent: a.ID})
			}
		}
		if a.Status == session.AgentRunning || seen && prev != session.AgentRunning {
			continue // not finished, or finished before
		}
		text := "done"
		if a.Status == session.AgentError {
			text = "failed"
		}
		lines = append(lines, LogLine{At: m.at(a.End), Actor: a.Name, Model: a.Model, Text: text})
		if m.primed {
			m.packets = append(m.packets, Packet{Agent: a.ID, Up: true})
		}
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].At.Before(lines[j].At) })
	m.st.Log = append(m.st.Log, lines...)
	m.st.Log = m.st.Log[max(len(m.st.Log)-maxLog, 0):]
	m.track(snap.Agents)
}

// at is t, or the frame's clock when t is unknown.
func (m *Model) at(t time.Time) time.Time {
	if t.IsZero() {
		return m.st.Now
	}
	return t
}

// track remembers each agent's status, to diff the next snapshot against.
func (m *Model) track(agents []session.Agent) {
	m.seen = make(map[string]session.AgentStatus, len(agents))
	for _, a := range agents {
		m.seen[a.ID] = a.Status
	}
	m.primed = true
}

// settle ends the animation: counters and the typewriter at their targets, no packets.
func (m *Model) settle() {
	lead := m.st.Snap.Lead
	m.turns, m.calls, m.tokens = lead.Turns, lead.Calls, lead.Tokens
	m.typed = float64(len([]rune(lead.LastText)))
	m.packets = nil
}

// Packets returns the packets on their way.
func (m Model) Packets() []Packet { return m.packets }

// frame is the state as drawn now: counters and the last text as far as the animation got.
func (m Model) frame() State {
	f := m.st
	f.Snap.Lead.Turns, f.Snap.Lead.Calls, f.Snap.Lead.Tokens = m.turns, m.calls, m.tokens
	text := []rune(f.Snap.Lead.LastText)
	f.Snap.Lead.LastText = string(text[:min(int(m.typed), len(text))])
	return f
}

// drawPackets draws each packet on its agent's connector: a head with a fading tail behind it. Packets of agents
// folded into a "+n more" box aren't drawn.
func (s State) drawPackets(c *Canvas, packets []Packet, nodes []Node, paths [][]Point) {
	node := map[string]int{}
	for i, n := range nodes {
		if n.Agent >= 0 {
			node[s.Snap.Agents[n.Agent].ID] = i
		}
	}
	for _, p := range packets {
		i, ok := node[p.Agent]
		if !ok || len(paths[i]) == 0 {
			continue
		}
		path, role := paths[i], ModelRole(s.Snap.Agents[nodes[i].Agent].Model)
		k := int(p.Pos * float64(len(path)-1))
		for back, r := range []rune{'·', '•', '●'} { // tail first, so the head wins
			j := k - (2 - back)
			if j < 0 {
				continue
			}
			if p.Up {
				j = len(path) - 1 - j
			}
			c.Set(path[j].X, path[j].Y, r, role)
		}
	}
}
