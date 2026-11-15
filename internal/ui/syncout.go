package ui

import (
	"io"
	"os"
	"os/exec"
	"sync"
)

// SyncOut is the terminal output of the program, shared with the background query so that the query never lands
// inside a frame: bubbletea writes each frame in one Write, and a query is written under the same lock.
type SyncOut struct {
	*os.File // the terminal; embedded so bubbletea still sees a terminal to size and read
	mu       sync.Mutex
	dst      io.Writer // where Write goes; the file unless a test swaps it
}

// NewSyncOut wraps the terminal file f.
func NewSyncOut(f *os.File) *SyncOut { return &SyncOut{File: f, dst: f} }

// Write writes p to the terminal.
func (o *SyncOut) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.dst.Write(p)
}

// With runs f while no frame is being written and none starts.
func (o *SyncOut) With(f func()) {
	o.mu.Lock()
	defer o.mu.Unlock()
	f()
}

// WithSyncOut makes the background query write under the lock of the program's output o.
func WithSyncOut(o *SyncOut) Option {
	return func(m *Model) { m.syncOut = o }
}

// onTerminal gives cmd the terminal file as stdout, for a command run through tea.ExecProcess. Left alone,
// bubbletea hands the child the program's output, which is not an *os.File here, so exec pipes it and the child
// (tmux attach: "open terminal failed: not a terminal") sees no terminal.
func (m *Model) onTerminal(cmd *exec.Cmd) *exec.Cmd {
	if cmd.Stdout == nil && m.syncOut != nil {
		cmd.Stdout = m.syncOut.File
	}
	return cmd
}
