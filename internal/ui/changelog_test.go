package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/update"
)

func rel(tag string) update.Release {
	return update.Release{Tag: tag, Body: "## Changelog\n### Features\n* abc1234: feat: note of " + tag + " (@someone)\n"}
}

// newLogModel returns a Model wired to a notes fetcher and a last_version file in a temp dir; calls counts
// the fetches. last "" leaves the file out.
func newLogModel(t *testing.T, current, last string, off bool, rels []update.Release, fetchErr error) (m *Model, path string, calls *int) {
	t.Helper()
	m, _ = newUpdateModel(t, current, latest("v9.9.9"), off, false)
	m.width, m.height = 100, 30
	path = filepath.Join(t.TempDir(), "last_version")
	if last != "" {
		if err := update.SaveLastRun(path, last); err != nil {
			t.Fatal(err)
		}
	}
	n := 0
	WithChangelog(func(context.Context) ([]update.Release, error) {
		n++
		return rels, fetchErr
	}, current, path)(m)
	return m, path, &n
}

// start runs the start-up changelog command, if any, and delivers its message.
func start(m *Model) {
	if cmd := m.changelogStart(); cmd != nil {
		m.Update(cmd())
	}
}

func lastVersion(path string) string {
	b, _ := os.ReadFile(path)
	return strings.TrimSpace(string(b))
}

func TestChangelogOnStart(t *testing.T) {
	all := []update.Release{rel("v1.3.0"), rel("v1.2.0"), rel("v1.1.0"), rel("v1.0.0")}
	many := make([]update.Release, 0, 14)
	for i := 14; i >= 1; i-- {
		many = append(many, rel(fmt.Sprintf("v1.%d.0", i)))
	}
	tests := []struct {
		name      string
		current   string
		last      string
		off       bool
		rels      []update.Release
		fetchErr  error
		wantCalls int
		wantShown []string // tags on screen, newest first; nil = no screen
		wantFile  string
	}{
		{"upgrade", "v1.2.0", "v1.0.0", false, all, nil, 1, []string{"v1.2.0", "v1.1.0"}, "v1.2.0"},
		{"no record writes the version", "v1.2.0", "", false, all, nil, 0, nil, "v1.2.0"},
		{"same version", "v1.2.0", "v1.2.0", false, all, nil, 0, nil, "v1.2.0"},
		{"fetch fails", "v1.2.0", "v1.0.0", false, nil, errors.New("offline"), 1, nil, "v1.0.0"},
		{"no_update_check", "v1.2.0", "v1.0.0", true, all, nil, 0, nil, "v1.0.0"},
		{"dev build", "dev", "v1.0.0", false, all, nil, 0, nil, "v1.0.0"},
		{"no release in range", "v1.2.0", "v1.0.0", false, []update.Release{rel("v1.3.0")}, nil, 1, nil, "v1.2.0"},
		{"capped at 10", "v1.14.0", "v1.0.0", false, many, nil, 1, []string{"v1.14.0", "v1.13.0", "v1.12.0", "v1.11.0", "v1.10.0", "v1.9.0", "v1.8.0", "v1.7.0", "v1.6.0", "v1.5.0"}, "v1.14.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, path, calls := newLogModel(t, tt.current, tt.last, tt.off, tt.rels, tt.fetchErr)
			start(m)
			if *calls != tt.wantCalls {
				t.Errorf("fetches = %d, want %d", *calls, tt.wantCalls)
			}
			if got := m.mode == modeChangelog; got != (tt.wantShown != nil) {
				t.Fatalf("changelog screen shown = %v, want %v", got, tt.wantShown != nil)
			}
			if tt.wantShown != nil {
				screen := ansi.Strip(strings.Join(m.changelogLines(), "\n"))
				if want := "What's new in lanes " + tt.current + " (since " + tt.last + ")"; !strings.Contains(screen, want) {
					t.Errorf("screen lacks title %q:\n%s", want, screen)
				}
				prev := -1
				for _, tag := range tt.wantShown {
					i := strings.Index(screen, tag+"\n")
					if i <= prev {
						t.Fatalf("%s missing or out of order in:\n%s", tag, screen)
					}
					prev = i
				}
				if !strings.Contains(screen, "note of "+tt.wantShown[0]) || strings.Contains(screen, "abc1234") || strings.Contains(screen, "@someone") {
					t.Errorf("notes not cleaned:\n%s", screen)
				}
				if n := strings.Count(screen, "note of"); n != len(tt.wantShown) {
					t.Errorf("%d releases on screen, want %d", n, len(tt.wantShown))
				}
			}
			if got := lastVersion(path); got != tt.wantFile {
				t.Errorf("last_version = %q, want %q", got, tt.wantFile)
			}
		})
	}
}

func TestChangelogWaitsForQuietScreen(t *testing.T) {
	m, path, _ := newLogModel(t, "v1.2.0", "v1.0.0", false, []update.Release{rel("v1.2.0")}, nil)
	m.mode = modeSearch
	start(m)
	if m.mode != modeSearch {
		t.Fatalf("mode = %v: the screen stole the search", m.mode)
	}
	if got := lastVersion(path); got != "v1.0.0" {
		t.Errorf("last_version = %q before the screen was shown", got)
	}
	m.mode = modeNormal
	st := m.fetchStateOf("p")
	st.running = true // the first list is not ready
	m.Update(usageMsg{})
	if m.mode == modeChangelog {
		t.Fatal("opened before the first list was ready")
	}
	st.running = false
	m.Update(usageMsg{})
	if m.mode != modeChangelog {
		t.Fatalf("mode = %v, want the changelog once the list is ready", m.mode)
	}
	if got := lastVersion(path); got != "v1.2.0" {
		t.Errorf("last_version = %q after the screen was shown", got)
	}
}

func TestChangelogCommand(t *testing.T) {
	rels := []update.Release{rel("v1.4.0"), rel("v1.3.0"), rel("v1.2.0"), rel("v1.1.0"), rel("v1.0.0")}
	for _, off := range []bool{false, true} {
		t.Run(fmt.Sprintf("no_update_check=%v", off), func(t *testing.T) {
			m, path, calls := newLogModel(t, "v1.3.0", "v1.3.0", off, rels, nil)
			_, cmd := m.runCommand("changelog")
			if cmd == nil {
				t.Fatal(":changelog returned no command")
			}
			m.Update(cmd())
			if *calls != 1 || m.mode != modeChangelog {
				t.Fatalf("fetches = %d, mode = %v", *calls, m.mode)
			}
			screen := ansi.Strip(strings.Join(m.changelogLines(), "\n"))
			for _, tag := range []string{"v1.3.0", "v1.2.0", "v1.1.0"} {
				if !strings.Contains(screen, "note of "+tag) {
					t.Errorf("screen lacks %s:\n%s", tag, screen)
				}
			}
			if strings.Contains(screen, "note of v1.4.0") || strings.Contains(screen, "note of v1.0.0") {
				t.Errorf("screen shows more than the 3 newest releases up to the running one:\n%s", screen)
			}
			if got := lastVersion(path); got != "v1.3.0" {
				t.Errorf("last_version = %q, :changelog must not change it", got)
			}
		})
	}
}

func TestChangelogCommandFailure(t *testing.T) {
	m, _, _ := newLogModel(t, "v1.3.0", "v1.3.0", false, nil, errors.New("offline"))
	_, cmd := m.runCommand("changelog")
	m.Update(cmd())
	if m.mode == modeChangelog || m.notice == "" {
		t.Errorf("mode = %v, notice = %q: want a notice and no screen", m.mode, m.notice)
	}
}

func TestChangelogKeys(t *testing.T) {
	var rels []update.Release
	for i := 20; i >= 1; i-- {
		rels = append(rels, rel(fmt.Sprintf("v1.%d.0", i)))
	}
	tests := []struct {
		name    string
		keys    []string
		wantOff int
		closed  bool
	}{
		{"j scrolls", []string{"j", "j"}, 2, false},
		{"k stops at the top", []string{"j", "k", "k"}, 0, false},
		{"q closes", []string{"q"}, 0, true},
		{"esc closes", []string{"j", "esc"}, 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _, _ := newLogModel(t, "v1.20.0", "v1.0.0", false, rels, nil)
			start(m)
			if m.mode != modeChangelog {
				t.Fatal("changelog not open")
			}
			m.View() // clamps the offset like a frame would
			press(m, tt.keys...)
			m.View()
			if got := m.mode != modeChangelog; got != tt.closed {
				t.Errorf("closed = %v, want %v", got, tt.closed)
			}
			if m.helpOff != tt.wantOff {
				t.Errorf("offset = %d, want %d", m.helpOff, tt.wantOff)
			}
		})
	}
}
