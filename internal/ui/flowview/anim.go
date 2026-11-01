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

	streamEvery = 1200 * time.Millisecond // a running edge sends a packet about this often, jittered per edge
	streamJit   = 400 * time.Millisecond  // the spread of the per-edge interval around streamEvery
	maxInFlight = 3                       // packets one edge streams at once; more are dropped
	burstSize   = 3                       // packets a finished or failed agent sends up its edge
	burstGap    = 0.2                     // the share of a trip between two packets of a burst
)

// toolEdge is the Agent of packets on the lead's connector to the tool layer. Agent ids are never empty.
const toolEdge = ""

// Packet is a dot travelling along a connector: down from the parent while the agent runs, up to the parent in a
// burst when it finishes or fails. Packets on the toolEdge run from the lead to the tool layer while it works.
type Packet struct {
	Agent string  // the agent whose connector it travels, or toolEdge
	Up    bool    // child to parent (finish) instead of parent to child
	Pos   float64 // progress along the connector, 0..1; a burst's later packets start below 0 and show from 0
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
	m.stream(dt)
	lead := m.st.Snap.Lead
	m.turns = ease(m.turns, lead.Turns, dt)
	m.calls = ease(m.calls, lead.Calls, dt)
	m.tokens = ease(m.tokens, lead.Tokens, dt)
	m.tokIn = ease(m.tokIn, lead.TokensIn, dt)
	m.tokOut = ease(m.tokOut, lead.TokensOut, dt)
	m.tokCache = ease(m.tokCache, lead.TokensCache, dt)
	m.typed = min(m.typed+typeRate*dt.Seconds(), float64(len([]rune(lead.LastText))))
}

// stream sends the packets due after dt on the streaming edges: the lead's to the tool layer while it works and
// each running agent's from its parent. An edge that starts streaming sends its first packet after its jitter, so
// edges don't move in lockstep; a packet due while the edge has maxInFlight on their way is dropped.
func (m *Model) stream(dt time.Duration) {
	ids := make([]string, 0, len(m.st.Snap.Agents)+1)
	if m.st.Hook == "working" {
		ids = append(ids, toolEdge)
	}
	for _, a := range m.st.Snap.Agents {
		if a.Status == session.AgentRunning {
			ids = append(ids, a.ID)
		}
	}
	due := make(map[string]time.Duration, len(ids))
	for _, id := range ids {
		next, ok := m.due[id]
		if !ok {
			next = jitter(id) % every(id)
		}
		for next -= dt; next <= 0; next += every(id) {
			if m.inFlight(id) < maxInFlight {
				m.packets = append(m.packets, Packet{Agent: id, Pos: float64(-next) / float64(packetTime)})
			}
		}
		due[id] = next
	}
	m.due = due
}

// jitter is a fixed pseudo-random duration of up to streamJit per edge.
func jitter(id string) time.Duration {
	h := uint32(2166136261) // FNV-1a
	for i := 0; i < len(id); i++ {
		h = (h ^ uint32(id[i])) * 16777619
	}
	return time.Duration(h%uint32(streamJit/time.Millisecond)) * time.Millisecond
}

// every is the edge's interval between two packets: streamEvery, jittered by up to half of streamJit.
func every(id string) time.Duration { return streamEvery - streamJit/2 + jitter(id) }

// drop removes the packets on their way down the edge.
func (m *Model) drop(id string) {
	var kept []Packet
	for _, p := range m.packets {
		if p.Agent != id || p.Up {
			kept = append(kept, p)
		}
	}
	m.packets = kept
}

// inFlight counts the packets on their way down the edge.
func (m *Model) inFlight(id string) int {
	n := 0
	for _, p := range m.packets {
		if p.Agent == id && !p.Up {
			n++
		}
	}
	return n
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

// Apply takes an event into the view's state: a snapshot sends a packet down for agents that spawned, a burst up
// for those that finished, and log lines for both since the last one, a hook event moves the checkpoint, the end event freezes the view.
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
				if m.due == nil {
					m.due = map[string]time.Duration{}
				}
				m.due[a.ID] = every(a.ID)
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
			m.drop(a.ID) // the burst replaces the stream: at most burstSize packets on the edge
			for i := 0; i < burstSize; i++ {
				m.packets = append(m.packets, Packet{Agent: a.ID, Up: true, Pos: -burstGap * float64(i)})
			}
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

// settle ends the animation: counters and the typewriter at their targets, no packets and no streams.
func (m *Model) settle() {
	lead := m.st.Snap.Lead
	m.turns, m.calls, m.tokens = lead.Turns, lead.Calls, lead.Tokens
	m.tokIn, m.tokOut, m.tokCache = lead.TokensIn, lead.TokensOut, lead.TokensCache
	m.typed = float64(len([]rune(lead.LastText)))
	m.packets, m.due = nil, nil
}

// Packets returns the packets on their way.
func (m Model) Packets() []Packet { return m.packets }

// frame is the state as drawn now: counters and the last text as far as the animation got.
func (m Model) frame() State {
	f := m.st
	f.Snap.Lead.Turns, f.Snap.Lead.Calls, f.Snap.Lead.Tokens = m.turns, m.calls, m.tokens
	f.Snap.Lead.TokensIn, f.Snap.Lead.TokensOut, f.Snap.Lead.TokensCache = m.tokIn, m.tokOut, m.tokCache
	text := []rune(f.Snap.Lead.LastText)
	f.Snap.Lead.LastText = string(text[:min(int(m.typed), len(text))])
	return f
}

// drawPackets draws each packet on its connector: a head with a fading tail behind it, in the sender's color: the
// parent's on the way down, the agent's on the way up. Packets further along are drawn last, so their heads win
// where connectors share cells. Packets of agents without a box, and burst packets not started yet, aren't drawn.
func (s State) drawPackets(c *Canvas, packets []Packet, nodes []Node, paths [][]Point, tools []Point) {
	node := map[string]int{}
	for i, n := range nodes {
		node[s.Snap.Agents[n.Agent].ID] = i
	}
	model := map[string]string{"": s.Snap.Lead.Model} // the senders' models by agent id, the lead's by ""
	for _, a := range s.Snap.Agents {
		model[a.ID] = a.Model
	}
	order := make([]Packet, 0, len(packets))
	for _, p := range packets {
		if p.Pos >= 0 {
			order = append(order, p)
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return order[i].Pos < order[j].Pos })
	for _, p := range order {
		var path []Point
		var role Role
		if p.Agent == toolEdge {
			path, role = tools, ModelRole(s.Snap.Lead.Model)
		} else {
			i, ok := node[p.Agent]
			if !ok {
				continue
			}
			a := s.Snap.Agents[nodes[i].Agent]
			path, role = paths[i], ModelRole(a.Model)
			if pm, ok := model[a.Parent]; !p.Up && ok {
				role = ModelRole(pm)
			} else if !p.Up {
				role = ModelRole(s.Snap.Lead.Model) // an unknown parent is the lead
			}
		}
		if len(path) == 0 {
			continue
		}
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
