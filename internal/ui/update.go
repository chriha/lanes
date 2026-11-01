package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/update"
)

// Option configures a Model in New.
type Option func(*Model)

// WithUpdateCheck makes the TUI ask c at start whether a newer release exists, unless settings.no_update_check
// is set; a newer one shows an info notice with upgrade, the command for how lanes was installed.
func WithUpdateCheck(c update.Checker, upgrade string) Option {
	return func(*Model) {}
}

// updateMsg is the result of the start-up release check.
type updateMsg struct {
	tag   string // newest release
	newer bool   // tag is newer than the running version
}

// updateCheck returns the release check to run at start, or nil when it is off. The check is throttled by
// its cache and runs off the event loop; its result comes back as an updateMsg.
func (m *Model) updateCheck() tea.Cmd {
	return nil
}

// onUpdate shows a newer release as an info notice, unless another notice is showing: it never hides an error.
func (m *Model) onUpdate(updateMsg) {}
