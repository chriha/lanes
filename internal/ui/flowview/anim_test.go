package flowview

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/session"
)

var t0 = time.Date(2026, 11, 1, 14, 0, 0, 0, time.UTC)

// liveModel is a 90×46 view of a working session whose lead spawned the running agent a1.
func liveModel() Model {
	m := New(nil)
	m.SetSize(90, 46)
	m.SetState(State{Title: "s", Hook: "working", Start: t0, Now: t0, Snap: session.Snapshot{
		Lead:   session.Lead{Model: "claude-opus-4-1"},
		Agents: []session.Agent{{ID: "a1", Name: "implementer", Model: "claude-opus-4-1", Start: t0, Ctx: -1}},
	}})
	return m
}

// withAgents returns the snapshot of m with agents appended.
func withAgents(m Model, agents ...session.Agent) session.Snapshot {
	snap := m.st.Snap
	snap.Agents = append(append([]session.Agent(nil), snap.Agents...), agents...)
	return snap
}

func TestSnapshotSendsPackets(t *testing.T) {
	child := session.Agent{ID: "a2", Parent: "a1", Name: "reviewer", Model: "claude-sonnet-4-5", Start: t0, Ctx: -1}
	done := func(s session.AgentStatus) session.Snapshot {
		m := liveModel()
		snap := m.st.Snap
		snap.Agents = []session.Agent{snap.Agents[0]}
		snap.Agents[0].Status, snap.Agents[0].End = s, t0.Add(time.Minute)
		return snap
	}
	tests := []struct {
		name string
		snap session.Snapshot
		want []Packet
	}{
		{"a spawned agent gets a packet down", withAgents(liveModel(), child), []Packet{{Agent: "a2"}}},
		{"a finished agent gets a packet up", done(session.AgentDone), []Packet{{Agent: "a1", Up: true}}},
		{"a failed agent gets a packet up", done(session.AgentError), []Packet{{Agent: "a1", Up: true}}},
		{"an unchanged agent gets none", liveModel().st.Snap, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := liveModel()
			m.Apply(SnapshotEvent{tt.snap})
			got := m.Packets()
			if len(got) != len(tt.want) {
				t.Fatalf("packets = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("packet %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestPacketsAdvanceAndEnd(t *testing.T) {
	m := liveModel()
	m.Apply(SnapshotEvent{withAgents(m, session.Agent{ID: "a2", Parent: "a1", Name: "reviewer", Start: t0, Ctx: -1})})
	steps := []struct {
		dt      time.Duration
		wantPos []float64
	}{
		{0, []float64{0}},
		{packetTime / 4, []float64{0.25}},
		{packetTime / 4, []float64{0.5}},
		{packetTime / 2, nil}, // at the end: gone
	}
	for i, st := range steps {
		m.Step(st.dt)
		got := m.Packets()
		if len(got) != len(st.wantPos) {
			t.Fatalf("step %d: packets = %v, want positions %v", i, got, st.wantPos)
		}
		for j := range got {
			if d := got[j].Pos - st.wantPos[j]; d > 1e-9 || d < -1e-9 {
				t.Errorf("step %d: pos = %v, want %v", i, got[j].Pos, st.wantPos[j])
			}
		}
	}
}

// cellOf returns the rune at x, y of the frame with escape sequences stripped.
func cellOf(frame string, x, y int) rune {
	lines := strings.Split(ansi.Strip(frame), "\n")
	if y >= len(lines) {
		return 0
	}
	r := []rune(lines[y])
	if x >= len(r) {
		return 0
	}
	return r[x]
}

func TestPacketIsDrawnOnItsConnector(t *testing.T) {
	for _, up := range []bool{false, true} {
		m := liveModel()
		a2 := session.Agent{ID: "a2", Parent: "a1", Name: "reviewer", Start: t0, Ctx: -1}
		m.Apply(SnapshotEvent{withAgents(m, a2)})
		if up {
			m.Step(packetTime) // the spawn packet is gone
			a2.Status, a2.End = session.AgentDone, t0.Add(time.Second)
			m.Apply(SnapshotEvent{withAgents(liveModel(), a2)})
		}
		m.Step(packetTime / 2)
		frame := m.View()

		l := m.layout
		nodes := layoutTree(m.st.Snap.Agents, l.Tree, l.Box)
		paths := treePaths(nodes, l.Tools)
		if len(paths) != 2 {
			t.Fatalf("up=%v: %d connectors, want 2", up, len(paths))
		}
		path := paths[1] // a2's connector from a1
		k := int(0.5 * float64(len(path)-1))
		head, tail := path[k], path[k-1]
		if up {
			head, tail = path[len(path)-1-k], path[len(path)-k]
		}
		if got := cellOf(frame, head.X, head.Y); got != '●' {
			t.Errorf("up=%v: head cell %v = %q, want ●", up, head, got)
		}
		if got := cellOf(frame, tail.X, tail.Y); got != '•' {
			t.Errorf("up=%v: tail cell %v = %q, want •", up, tail, got)
		}
	}
}

func TestCountersEaseToTheirTarget(t *testing.T) {
	tests := []struct {
		name       string
		from, to   int
		wantFirst  int // after the first 100 ms step
		wantSteps  int // steps of 100 ms until the target is reached
		wantTarget int
	}{
		{"up by a lot rolls", 0, 1000, 250, 25, 1000},
		{"up by little moves one per step", 0, 2, 1, 2, 2},
		{"down eases too", 1000, 0, 750, 25, 0},
		{"at target stays", 5, 5, 5, 0, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := liveModel()
			snap := m.st.Snap
			snap.Lead.Tokens = tt.from
			m.SetState(State{Now: t0, Snap: snap})
			snap.Lead.Tokens = tt.to
			m.Apply(SnapshotEvent{snap})
			if got := m.frame().Snap.Lead.Tokens; got != tt.from {
				t.Fatalf("before a step: shown %d, want %d", got, tt.from)
			}
			m.Step(100 * time.Millisecond)
			if got := m.frame().Snap.Lead.Tokens; got != tt.wantFirst {
				t.Errorf("after 100ms: shown %d, want %d", got, tt.wantFirst)
			}
			steps, prev := 1, m.frame().Snap.Lead.Tokens
			for ; prev != tt.to && steps < 100; steps++ {
				m.Step(100 * time.Millisecond)
				got := m.frame().Snap.Lead.Tokens
				if (tt.to-got)*(tt.to-tt.from) < 0 || (got-prev)*(tt.to-tt.from) < 0 {
					t.Fatalf("step %d: shown %d after %d overshoots or turns back toward %d", steps, got, prev, tt.to)
				}
				prev = got
			}
			if tt.from == tt.to {
				steps = 0
			}
			if prev != tt.wantTarget || steps != tt.wantSteps {
				t.Errorf("shown %d after %d steps, want %d after %d", prev, steps, tt.wantTarget, tt.wantSteps)
			}
		})
	}
}

func TestTypewriter(t *testing.T) {
	m := liveModel()
	snap := m.st.Snap
	snap.Lead.LastText = "hello world"
	m.Apply(SnapshotEvent{snap})
	steps := []struct {
		name string
		do   func()
		want string
	}{
		{"a new text starts empty", func() {}, ""},
		{"it types 40 runes a second", func() { m.Step(100 * time.Millisecond) }, "hell"},
		{"it stops at the end", func() { m.Step(time.Second) }, "hello world"},
		{"the same text again keeps it", func() { m.Apply(SnapshotEvent{snap}) }, "hello world"},
		{"a changed text starts over", func() {
			snap.Lead.LastText = "bye"
			m.Apply(SnapshotEvent{snap})
			m.Step(50 * time.Millisecond)
		}, "by"},
	}
	for _, st := range steps {
		st.do()
		if got := m.frame().Snap.Lead.LastText; got != st.want {
			t.Errorf("%s: shown %q, want %q", st.name, got, st.want)
		}
	}
}

func TestHookEventMovesTheCheckpoint(t *testing.T) {
	for _, hook := range hookStates {
		m := liveModel()
		m.Apply(HookEvent{hook})
		frame := ansi.Strip(m.View())
		for _, h := range hookStates {
			mark := "◇ " + h
			if h == hook {
				mark = "◆ " + h
			}
			if !strings.Contains(frame, mark) {
				t.Errorf("hook %s: frame lacks %q", hook, mark)
			}
		}
	}
}

func TestSpinnerAndShimmerAdvance(t *testing.T) {
	m := liveModel()
	steps := []struct {
		dt   time.Duration
		want int
	}{
		{0, 0},
		{phaseEvery / 2, 0},
		{phaseEvery / 2, 1},
		{3 * phaseEvery, 4},
		{phaseEvery - 1, 4},
	}
	for i, st := range steps {
		m.Step(st.dt)
		if got := m.frame().Phase; got != st.want {
			t.Errorf("step %d: phase %d, want %d", i, got, st.want)
		}
	}
	if m.frame().spin() != spinner[0] {
		t.Errorf("spinner after 4 phases = %q, want %q", m.frame().spin(), spinner[0])
	}
}

func TestEndFreezesTheFrame(t *testing.T) {
	m := liveModel()
	snap := withAgents(m, session.Agent{ID: "a2", Parent: "a1", Name: "reviewer", Start: t0, Ctx: -1})
	snap.Lead.Turns, snap.Lead.LastText = 40, "all done"
	m.Apply(SnapshotEvent{snap})
	m.Step(100 * time.Millisecond)
	m.Apply(EndEvent{})
	f := m.frame()
	if !f.Ended || len(m.Packets()) != 0 || f.Snap.Lead.Turns != 40 || f.Snap.Lead.LastText != "all done" {
		t.Fatalf("after the end: ended %v, packets %v, turns %d, text %q; want ended, none, 40, all done",
			f.Ended, m.Packets(), f.Snap.Lead.Turns, f.Snap.Lead.LastText)
	}
	phase, now := f.Phase, f.Now
	m.Step(time.Second)
	if f := m.frame(); f.Phase != phase || !f.Now.Equal(now) {
		t.Errorf("an ended view moved: phase %d→%d, now %v→%v", phase, f.Phase, now, f.Now)
	}
	snap.Lead.Turns = 41
	m.Apply(SnapshotEvent{snap}) // a last poll after the end
	if got := m.frame().Snap.Lead.Turns; got != 41 {
		t.Errorf("a snapshot after the end shows %d turns, want 41 at once", got)
	}
}

func TestSnapshotLogsSpawnsAndFinishes(t *testing.T) {
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }
	agents := []session.Agent{
		{ID: "a1", Name: "implementer", Model: "claude-opus-4-1", Start: at(1), Status: session.AgentDone, End: at(5)},
		{ID: "a2", Parent: "a1", Name: "reviewer", Model: "claude-sonnet-4-5", Start: at(2), Status: session.AgentError, End: at(3)},
		{ID: "a3", Name: "triager", Model: "claude-haiku-4-5", Start: at(4)},
	}
	want := []string{
		"14:00:01 lead spawned implementer",
		"14:00:02 implementer spawned reviewer",
		"14:00:03 reviewer failed",
		"14:00:04 lead spawned triager",
		"14:00:05 implementer done",
	}
	m := New(nil) // nothing set yet: the first snapshot is history, without packets
	m.Apply(SnapshotEvent{session.Snapshot{Lead: session.Lead{Model: "claude-opus-4-1"}, Agents: agents}})
	var got []string
	for _, l := range m.st.Log {
		got = append(got, l.At.Format("15:04:05")+" "+l.Actor+" "+l.Text)
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("log:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(m.Packets()) != 0 {
		t.Errorf("history sent packets: %v", m.Packets())
	}
}

func TestTickRunsOnlyWhileOpenAndLive(t *testing.T) {
	tests := []struct {
		name      string
		prep      func(m *Model) tickMsg // returns the tick to deliver
		wantCmd   bool
		wantPhase int
	}{
		{"own tick steps and re-arms", func(m *Model) tickMsg {
			return tickMsg{id: m.id, gen: m.gen, t: t0}
		}, true, 0},
		{"own tick after a tick steps by the time between", func(m *Model) tickMsg {
			m.last = t0
			m.phaseClock = phaseEvery / 2
			return tickMsg{id: m.id, gen: m.gen, t: t0.Add(phaseEvery*3/2 + time.Millisecond)}
		}, true, 2},
		{"a long gap steps at most maxStep", func(m *Model) tickMsg {
			m.last = t0
			return tickMsg{id: m.id, gen: m.gen, t: t0.Add(time.Hour)}
		}, true, int(maxStep / phaseEvery)}, // not an hour's worth
		{"another model's tick is ignored", func(m *Model) tickMsg {
			m.last = t0
			return tickMsg{id: m.id + 1, gen: m.gen, t: t0.Add(time.Second)}
		}, false, 0},
		{"paused: no step, no re-arm", func(m *Model) tickMsg {
			m.last = t0
			m.Pause()
			return tickMsg{id: m.id, gen: m.gen, t: t0.Add(time.Second)}
		}, false, 0},
		{"a tick from before a resume is dropped", func(m *Model) tickMsg {
			m.last = t0
			old := tickMsg{id: m.id, gen: m.gen, t: t0.Add(time.Second)}
			m.Pause()
			if m.Resume() == nil {
				t.Error("resume of a live view returned no tick")
			}
			return old
		}, false, 0},
		{"ended: no step, no re-arm", func(m *Model) tickMsg {
			m.last = t0
			m.Apply(EndEvent{})
			return tickMsg{id: m.id, gen: m.gen, t: t0.Add(time.Second)}
		}, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := liveModel()
			msg := tt.prep(&m)
			m, cmd := m.Update(msg)
			if (cmd != nil) != tt.wantCmd {
				t.Errorf("re-armed = %v, want %v", cmd != nil, tt.wantCmd)
			}
			if m.st.Phase != tt.wantPhase {
				t.Errorf("phase = %d, want %d", m.st.Phase, tt.wantPhase)
			}
		})
	}
}

func TestTickKeepsTheClockAfterAStall(t *testing.T) {
	m := liveModel()
	m.last = t0
	m, _ = m.Update(tickMsg{id: m.id, gen: m.gen, t: t0.Add(time.Hour)})
	if want := t0.Add(time.Hour); !m.st.Now.Equal(want) {
		t.Errorf("clock after a stalled tick = %v, want %v", m.st.Now, want)
	}
}

func TestInitAndResumeStartTheTickOnlyWhenLive(t *testing.T) {
	m := liveModel()
	if m.Init() == nil {
		t.Error("Init of a live view returned no tick")
	}
	m.Pause()
	if m.Init() != nil {
		t.Error("Init of a paused view returned a tick")
	}
	m.Apply(EndEvent{})
	if m.Resume() != nil {
		t.Error("Resume of an ended view returned a tick")
	}
	if m.Init() != nil {
		t.Error("Init of an ended view returned a tick")
	}
}

func TestUpdateAppliesEvents(t *testing.T) {
	m := liveModel()
	m, cmd := m.Update(HookEvent{"asking"})
	if m.st.Hook != "asking" || cmd != nil {
		t.Errorf("after a hook event: hook %q, cmd %v; want asking, nil", m.st.Hook, cmd != nil)
	}
}

func BenchmarkView(b *testing.B) {
	m := New(NewTheme(testPalette()))
	m.SetSize(90, 46)
	st := fixtureState()
	running := st.Snap.Agents[2]
	st.Snap.Agents = st.Snap.Agents[:2]
	m.SetState(st)
	st.Snap.Agents = append(st.Snap.Agents, running)
	m.Apply(SnapshotEvent{st.Snap}) // a packet on its way
	m.Step(packetTime / 3)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = m.View()
	}
}
