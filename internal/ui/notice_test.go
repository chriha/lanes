package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/config"
)

func TestNoticeLevels(t *testing.T) {
	cfgFile := func(t *testing.T, body string) {
		t.Helper()
		if err := os.MkdirAll(app.ConfigDir(), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(app.ConfigDir(), "config.toml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name string
		do   func(t *testing.T, m *Model)
		want level
	}{
		{"config error", func(t *testing.T, m *Model) {
			cfgFile(t, "not [valid")
			m.reload()
		}, lvErr},
		{"tmux unavailable", func(t *testing.T, m *Model) { m.sess = nil; m.runCommand("prune") }, lvWarn},
		{"unknown command", func(t *testing.T, m *Model) { m.runCommand("bogus") }, lvWarn},
		{"built-in key ignored on reload", func(t *testing.T, m *Model) {
			cfgFile(t, "[action.quick]\ntype = \"run\"\nrun = \"true\"\n[keymap]\nj = \"quick\"\n")
			m.reload()
			if !strings.Contains(m.notice, "built in and ignored") {
				t.Fatalf("notice = %q", m.notice)
			}
		}, lvWarn},
		{"failed run", func(t *testing.T, m *Model) { finish(t, m, m.runAction("broken", busyActions["broken"])) }, lvErr},
		{"successful run", func(t *testing.T, m *Model) { finish(t, m, m.runAction("quick", busyActions["quick"])) }, lvInfo},
		{"run skipping a busy item", func(t *testing.T, m *Model) {
			m.marks["1"], m.marks["2"] = true, true
			m.runAction("quick", busyActions["quick"])
			m.confirm.do() // locks 1 and 2
			m.marks = map[string]bool{"2": true, "3": true}
			m.runAction("quick", busyActions["quick"]) // asks for confirmation
			finish(t, m, m.confirm.do())
		}, lvWarn},
		{"copied", func(t *testing.T, m *Model) { m.clip = func(string) error { return nil }; press(m, "y") }, lvInfo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := busyModel(t)
			m.ov = &config.Overrides{Filters: map[string]string{}}
			tt.do(t, m)
			if m.notice == "" || m.level != tt.want {
				t.Errorf("notice %q has level %v, want %v", m.notice, m.level, tt.want)
			}
		})
	}
}

func TestNoticeColour(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(0) // termenv.TrueColor: render the colors the test compares
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	tests := []struct {
		name  string
		level level
		want  lipgloss.Style
	}{
		{"error", lvErr, sErr},
		{"warning", lvWarn, sWarn},
		{"info", lvInfo, sText},
	}
	for _, tt := range tests {
		for _, screen := range []string{"main", "sessions"} {
			t.Run(tt.name+" "+screen, func(t *testing.T) {
				m, _ := busyModel(t)
				m.say(tt.level, "something")
				render := m.footer
				if screen == "sessions" {
					m.openSessions()
					m.say(tt.level, "something")
					render = m.sessionsView
				}
				want := tt.want.Render(" something")
				if got := render(); !strings.Contains(got, want) {
					t.Errorf("%s footer = %q, want it to contain %q", screen, got, want)
				}
				if tt.want.GetForeground() == sText.GetForeground() == (tt.level != lvInfo) {
					t.Error("test styles are not distinct")
				}
			})
		}
	}
}
