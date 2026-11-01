package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/session"
	"github.com/chriha/lanes/internal/ui/flowview"
)

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
}

// agentPollMsg says it is time for the agent view's next read of the transcripts.
type agentPollMsg struct{ gen int }

// agentSnapMsg carries what a read of the transcripts gave.
type agentSnapMsg struct {
	gen  int
	snap session.Snapshot
	err  error
}

// poll reads what the transcripts gained since the last read, off the UI goroutine.
func (a *agentView) poll() tea.Cmd { return nil }
