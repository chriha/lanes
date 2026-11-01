package flowview

import (
	"strings"
	"testing"
	"time"

	"github.com/chriha/lanes/internal/session"
)

// cue is one event of a scripted session, at a time from the script's start.
type cue struct {
	at time.Duration
	ev Event
}

// script is a scripted event source: a session in which the lead spawns an implementer, which spawns a reviewer
// that fails, then finishes, and Claude exits. It stands in for the tailer in tests.
func script() []cue {
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }
	lead := func(turns, calls int, text string) session.Lead {
		return session.Lead{Model: "claude-opus-4-1", Turns: turns, Calls: calls, Tokens: turns * 1000,
			ToolCalls: map[string]int{"Agent": calls}, LastText: text}
	}
	impl := session.Agent{ID: "a1", Name: "implementer", Model: "claude-opus-4-1", Start: at(1), Ctx: 10}
	rev := session.Agent{ID: "a2", Parent: "a1", Name: "reviewer", Model: "claude-sonnet-4-5", Start: at(3), Ctx: 5}
	revFailed, implDone := rev, impl
	revFailed.Status, revFailed.End = session.AgentError, at(4)
	implDone.Status, implDone.End = session.AgentDone, at(6)
	return []cue{
		{0, HookEvent{"working"}},
		{1 * time.Second, SnapshotEvent{session.Snapshot{Lead: lead(2, 1, "spawning the implementer"), Agents: []session.Agent{impl}}}},
		{3 * time.Second, SnapshotEvent{session.Snapshot{Lead: lead(3, 1, "waiting"), Agents: []session.Agent{impl, rev}}}},
		{4 * time.Second, SnapshotEvent{session.Snapshot{Lead: lead(3, 1, "waiting"), Agents: []session.Agent{impl, revFailed}}}},
		{6 * time.Second, SnapshotEvent{session.Snapshot{Lead: lead(5, 2, "slice shipped"), Agents: []session.Agent{implDone, revFailed}}}},
		{7 * time.Second, HookEvent{"waiting"}},
		{9 * time.Second, EndEvent{}},
	}
}

// play replays the script into m with fixed steps of dt and returns the most packets on the way at once.
func play(m *Model, cues []cue, dt time.Duration) int {
	most := 0
	for clock := time.Duration(0); len(cues) > 0; clock += dt {
		for len(cues) > 0 && cues[0].at <= clock {
			m.Apply(cues[0].ev)
			cues = cues[1:]
		}
		most = max(most, len(m.Packets()))
		m.Step(dt)
	}
	return most
}

func TestScriptedSession(t *testing.T) {
	m := New(nil)
	m.SetSize(90, 46)
	m.SetState(State{Title: "scripted", Start: t0, Now: t0})
	most := play(&m, script(), tickEvery)

	f := m.frame()
	if !f.Ended || f.Hook != "waiting" {
		t.Errorf("ended %v, hook %q; want ended, waiting", f.Ended, f.Hook)
	}
	if most != 1 {
		t.Errorf("most packets at once = %d, want 1 (each event's packet lands before the next)", most)
	}
	if len(m.Packets()) != 0 {
		t.Errorf("packets left after the end: %v", m.Packets())
	}
	if l := f.Snap.Lead; l.Turns != 5 || l.Calls != 2 || l.Tokens != 5000 || l.LastText != "slice shipped" {
		t.Errorf("lead shown %+v, want the final counters and text", l)
	}
	var log []string
	for _, l := range f.Log {
		log = append(log, l.Actor+" "+l.Text)
	}
	want := "lead spawned implementer\nimplementer spawned reviewer\nreviewer failed\nimplementer done"
	if got := strings.Join(log, "\n"); got != want {
		t.Errorf("log:\n%s\nwant:\n%s", got, want)
	}
	// The frozen frame stays put.
	before := m.View()
	m.Step(time.Second)
	if m.View() != before {
		t.Error("the ended view changed after a step")
	}
}
