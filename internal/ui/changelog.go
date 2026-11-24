package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/update"
)

// WithChangelog makes the TUI show the release notes since the version that last ran.
func WithChangelog(fetch func(context.Context) ([]update.Release, error), current, lastPath string) Option {
	return func(*Model) {}
}

// changelogMsg is the result of a release-notes fetch.
type changelogMsg struct {
	rels     []update.Release
	since    string
	err      error
	saveErr  error
	onDemand bool
}

func (m *Model) changelogStart() tea.Cmd { return nil }
func (m *Model) changelogLines() []string { return nil }
