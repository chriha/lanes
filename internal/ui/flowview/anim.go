package flowview

import "time"

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
func (m *Model) Step(dt time.Duration) {}

// Apply takes an event into the view's state.
func (m *Model) Apply(ev Event) {}

// Packets returns the packets on their way.
func (m Model) Packets() []Packet { return nil }

// frame is the state as drawn now.
func (m Model) frame() State { return m.st }
