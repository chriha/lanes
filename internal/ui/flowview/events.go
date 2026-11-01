package flowview

import "github.com/chriha/lanes/internal/session"

// Event is a change of the session the view shows. The parent sends events to Update (or Apply); the view turns
// them into state, log lines and packets. Events are tea messages, so the parent can return them from a tea.Cmd.
type Event interface{ isEvent() }

// SnapshotEvent carries the session's newest transcript state, as the tailer read it.
type SnapshotEvent struct{ Snap session.Snapshot }

// HookEvent carries Claude's newest hook state: working, asking or waiting.
type HookEvent struct{ Hook string }

// EndEvent says Claude exited or the session finished: the view freezes on its final state.
type EndEvent struct{}

func (SnapshotEvent) isEvent() {}
func (HookEvent) isEvent()     {}
func (EndEvent) isEvent()      {}
