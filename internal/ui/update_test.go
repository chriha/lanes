package ui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/update"
)

// newUpdateModel returns a Model whose release check uses fetch through a cache in a temp dir; calls counts
// the fetches. off sets settings.no_update_check, check=false leaves WithUpdateCheck out.
func newUpdateModel(t *testing.T, current string, fetch func() (string, error), off, check bool) (*Model, *int) {
	t.Helper()
	fakeTmux(t) // New needs a tmux on PATH; macOS CI runners have none
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "HOME"} {
		t.Setenv(k, t.TempDir())
	}
	calls := 0
	c := update.Checker{
		Current: current,
		Fetch: func(context.Context) (string, error) {
			calls++
			return fetch()
		},
		Now:       func() time.Time { return time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC) },
		CachePath: filepath.Join(t.TempDir(), "update.json"),
	}
	cfg := &config.Config{
		Settings: config.Settings{NoUpdateCheck: off},
		Provider: map[string]config.Provider{"p": {Type: "command", Command: "true"}},
		View:     []config.View{{Name: "v", Provider: "p", Filter: "true"}},
	}
	var opts []Option
	if check {
		opts = append(opts, WithUpdateCheck(c, update.Brew))
	}
	m := New(cfg, opts...)
	if m.notice != "" {
		t.Fatalf("New left a notice: %q", m.notice)
	}
	return m, &calls
}

func latest(tag string) func() (string, error) { return func() (string, error) { return tag, nil } }

func TestUpdateNotice(t *testing.T) {
	tests := []struct {
		name      string
		current   string
		fetch     func() (string, error)
		wantCalls int
		want      string // the footer notice; "" = none
	}{
		{"newer release", "v1.0.0", latest("v1.1.0"), 1, "lanes v1.1.0 available · brew upgrade lanes"},
		{"same release", "v1.1.0", latest("v1.1.0"), 1, ""},
		{"older release", "v1.2.0", latest("v1.1.0"), 1, ""},
		{"failed fetch stays silent", "v1.0.0", func() (string, error) { return "", errors.New("offline") }, 1, ""},
		{"dev build never checks", "dev", latest("v1.1.0"), 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, calls := newUpdateModel(t, tt.current, tt.fetch, false, true)
			cmd := m.updateCheck()
			if cmd == nil {
				t.Fatal("updateCheck = nil, want a check")
			}
			m.Update(cmd())
			if *calls != tt.wantCalls {
				t.Errorf("fetches = %d, want %d", *calls, tt.wantCalls)
			}
			if m.notice != tt.want {
				t.Errorf("notice = %q, want %q", m.notice, tt.want)
			}
			if tt.want != "" && m.level != lvInfo {
				t.Errorf("level = %v, want info", m.level)
			}
		})
	}
}

func TestUpdateNoticeKeepsExistingNotice(t *testing.T) {
	tests := []struct {
		name  string
		level level
	}{
		{"error", lvErr},
		{"warning", lvWarn},
		{"info", lvInfo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newUpdateModel(t, "v1.0.0", latest("v1.1.0"), false, true)
			m.say(tt.level, "config: boom")
			cmd := m.updateCheck()
			if cmd == nil {
				t.Fatal("updateCheck = nil, want a check")
			}
			m.Update(cmd())
			if m.notice != "config: boom" || m.level != tt.level {
				t.Errorf("notice = %q (level %v), want the earlier %q (level %v)", m.notice, m.level, "config: boom", tt.level)
			}
		})
	}
}

func TestUpdateCheckOff(t *testing.T) {
	tests := []struct {
		name       string
		off, check bool
	}{
		{"no_update_check", true, true},
		{"no checker", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, calls := newUpdateModel(t, "v1.0.0", latest("v1.1.0"), tt.off, tt.check)
			if cmd := m.updateCheck(); cmd != nil {
				t.Error("updateCheck != nil, want no check")
			}
			if *calls != 0 {
				t.Errorf("fetches = %d, want 0", *calls)
			}
		})
	}
}

func TestUpdateBadge(t *testing.T) {
	tests := []struct {
		name    string
		current string
		off     bool
		check   bool
		want    string // "" = no badge
	}{
		{"newer release", "v1.0.0", false, true, "↑ v1.1.0"},
		{"same release", "v1.1.0", false, true, ""},
		{"older release", "v1.2.0", false, true, ""},
		{"dev build", "dev", false, true, ""},
		{"no_update_check", "v1.0.0", true, true, ""},
		{"no checker", "v1.0.0", false, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newUpdateModel(t, tt.current, latest("v1.1.0"), tt.off, tt.check)
			m.width = 120
			if cmd := m.updateCheck(); cmd != nil {
				m.Update(cmd())
			}
			h := ansi.Strip(m.header())
			if got := strings.Contains(h, "↑"); got != (tt.want != "") {
				t.Fatalf("header %q: badge present = %v, want %v", h, got, tt.want != "")
			}
			if tt.want != "" && !strings.Contains(h, tt.want) {
				t.Errorf("header %q lacks %q", h, tt.want)
			}
		})
	}
}

func TestUpdateBadgeSurvivesNotice(t *testing.T) {
	m, _ := newUpdateModel(t, "v1.0.0", latest("v1.1.0"), false, true)
	m.width = 120
	m.say(lvErr, "config: boom")
	m.Update(m.updateCheck()())
	if h := ansi.Strip(m.header()); !strings.Contains(h, "↑ v1.1.0") {
		t.Errorf("header %q lacks the badge while an error notice shows", h)
	}
}
