package ui

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/update"
)

// newUpdateModel returns a Model whose release check uses fetch through a cache in a temp dir; calls counts
// the fetches. off sets settings.no_update_check, check=false leaves WithUpdateCheck out.
func newUpdateModel(t *testing.T, current string, fetch func() (string, error), off, check bool) (*Model, *int) {
	t.Helper()
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
			m.Update(m.updateCheck()())
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
