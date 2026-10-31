package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/trust"
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
	project := func(t *testing.T) string {
		t.Helper()
		dir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, config.ProjectFile), []byte("# a\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		hash, err := trust.HashRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := trust.Trust(dir, hash); err != nil {
			t.Fatal(err)
		}
		old, _ := os.Getwd()
		t.Cleanup(func() { os.Chdir(old) })
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	tests := []struct {
		name string
		do   func(t *testing.T, m *Model)
		want level
	}{
		{"config error", func(t *testing.T, m *Model) {
			project(t)
			cfgFile(t, "not [valid")
			m.reload()
		}, lvErr},
		{"untrusted project change", func(t *testing.T, m *Model) {
			dir := project(t)
			if err := os.WriteFile(filepath.Join(dir, config.ProjectFile), []byte("# b\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			m.reload()
			if !strings.Contains(m.notice, "restart lanes to trust it") {
				t.Fatalf("notice = %q", m.notice)
			}
		}, lvWarn},
		{"trust state unreadable", func(t *testing.T, m *Model) {
			project(t)
			if err := os.WriteFile(filepath.Join(app.StateRoot(), "trust.json"), []byte("not json"), 0o644); err != nil {
				t.Fatal(err)
			}
			m.reload()
			if !strings.HasPrefix(m.notice, "trust: ") {
				t.Fatalf("notice = %q", m.notice)
			}
		}, lvErr},
		{"tmux unavailable", func(t *testing.T, m *Model) { m.sess = nil; m.runCommand("prune") }, lvWarn},
		{"unknown command", func(t *testing.T, m *Model) { m.runCommand("bogus") }, lvWarn},
		{"built-in key ignored on reload", func(t *testing.T, m *Model) {
			project(t)
			cfgFile(t, "[provider.p]\ntype = \"command\"\ncommand = \"true\"\n[[view]]\nname = \"v\"\nprovider = \"p\"\n[action.quick]\ntype = \"run\"\nrun = \"true\"\n[keymap]\nj = \"quick\"\n")
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
