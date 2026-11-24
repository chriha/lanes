package ui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/update"
)

const (
	changelogMax    = 10 // releases the start-up screen lists at most
	changelogDemand = 3  // releases :changelog lists when nothing is pending
)

// changelogDoc is what the changelog screen shows.
type changelogDoc struct {
	title string
	rels  []update.Release
}

// WithChangelog makes the TUI show the release notes since the version that last ran (recorded at lastPath),
// fetched with fetch, unless settings.no_update_check is set or current is not a release. :changelog works either way.
func WithChangelog(fetch func(context.Context) ([]update.Release, error), current, lastPath string) Option {
	return func(m *Model) { m.notesFetch, m.version, m.lastPath = fetch, current, lastPath }
}

// changelogMsg is the result of a release-notes fetch.
type changelogMsg struct {
	rels     []update.Release // newest first
	since    string           // the version that last ran; "" on demand
	saveErr  error            // writing last_version failed
	onDemand bool
}

// changelogStart returns the start-up check, or nil when it is off: no last_version → record the running version;
// a different one → fetch the notes in between. A failed fetch stays silent and leaves last_version, so the next
// start asks again.
func (m *Model) changelogStart() tea.Cmd {
	if m.notesFetch == nil || m.cfg.Settings.NoUpdateCheck || !update.Valid(m.version) {
		return nil
	}
	fetch, current, path := m.notesFetch, m.version, m.lastPath
	return func() tea.Msg {
		last, ok := update.LastRun(path)
		if !ok {
			return changelogMsg{saveErr: update.SaveLastRun(path, current)}
		}
		if last == current {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), update.Timeout)
		defer cancel()
		rels, err := fetch(ctx)
		if err != nil {
			return changelogMsg{}
		}
		rels = update.Between(rels, last, current, changelogMax)
		if len(rels) == 0 { // nothing to tell (a downgrade, or releases without notes): don't ask again
			return changelogMsg{saveErr: update.SaveLastRun(path, current)}
		}
		return changelogMsg{rels: rels, since: last}
	}
}

// changelogOnDemand fetches the newest releases up to the running one for :changelog.
func (m *Model) changelogOnDemand() tea.Cmd {
	if m.notesFetch == nil {
		m.say(lvWarn, "release notes unavailable")
		return nil
	}
	fetch, current := m.notesFetch, m.version
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), update.Timeout)
		defer cancel()
		rels, err := fetch(ctx)
		if err != nil {
			return changelogMsg{onDemand: true, saveErr: err}
		}
		return changelogMsg{onDemand: true, rels: update.Between(rels, "v0.0.0", current, changelogDemand)}
	}
}

func (m *Model) onChangelog(msg changelogMsg) {
	if msg.onDemand {
		switch {
		case msg.saveErr != nil:
			m.say(lvWarn, "release notes: "+msg.saveErr.Error())
		case len(msg.rels) == 0:
			m.say(lvInfo, "no release notes for "+m.version)
		default:
			m.clog = &changelogDoc{title: fmt.Sprintf("Release notes of %s %s", app.Name, m.version), rels: msg.rels}
			m.openChangelog()
		}
		return
	}
	if msg.saveErr != nil {
		m.say(lvWarn, "saving last version: "+msg.saveErr.Error())
	}
	if len(msg.rels) > 0 {
		m.clog = &changelogDoc{title: fmt.Sprintf("What's new in %s %s (since %s)", app.Name, m.version, msg.since), rels: msg.rels}
		m.clogPending = true
	}
}

// openPendingChangelog shows the start-up screen once the list is ready and nothing else is on screen.
func (m *Model) openPendingChangelog() {
	if !m.clogPending || m.mode != modeNormal {
		return
	}
	for name := range m.providers {
		if m.fetching(name) {
			return
		}
	}
	m.openChangelog()
}

// openChangelog shows the screen; a start-up one is then marked as seen.
func (m *Model) openChangelog() {
	m.mode, m.helpOff = modeChangelog, 0
	if !m.clogPending {
		return
	}
	m.clogPending = false
	if err := update.SaveLastRun(m.lastPath, m.version); err != nil {
		m.say(lvWarn, "saving last version: "+err.Error())
	}
}

// changelogLines builds the changelog screen for the current width.
func (m *Model) changelogLines() []string {
	if m.clog == nil {
		return nil
	}
	w := max(m.width-2, 1)
	out := []string{sTitle.Render(" " + m.clog.title), ""}
	for _, r := range m.clog.rels {
		out = append(out, sTitle.Render(" "+r.Tag))
		for _, l := range strings.Split(strings.TrimSpace(update.Clean(r.Body)), "\n") {
			out = append(out, strings.Split(ansi.Wrap("  "+l, w, ""), "\n")...)
		}
		out = append(out, "")
	}
	return out
}
