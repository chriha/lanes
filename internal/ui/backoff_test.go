package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chriha/lanes/internal/cache"
	"github.com/chriha/lanes/internal/config"
)

func newBackoffModel(t *testing.T) *Model {
	t.Helper()
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "HOME"} {
		t.Setenv(k, t.TempDir())
	}
	m := New(&config.Config{
		Provider: map[string]config.Provider{"p": {Type: "command", Command: "true"}},
		View:     []config.View{{Name: "v", Provider: "p", Filter: "true"}},
	})
	m.width, m.height = 100, 40
	return m
}

func TestFailedFetchBacksOff(t *testing.T) {
	boom := fetchedMsg{name: "p", err: errors.New("boom")}
	tests := []struct {
		name     string
		failures int
		want     string // in the error line; default poll is 30s
	}{
		{"first failure waits poll_interval", 1, "retry in 30s"},
		{"second doubles", 2, "retry in 1m0s"},
		{"fourth", 4, "retry in 4m0s"},
		{"capped at 5 minutes", 9, "retry in 5m0s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBackoffModel(t)
			for i := 0; i < tt.failures; i++ {
				setFetching(m, "p", true)
				m.Update(boom)
			}
			if cmd := m.onTick(); cmd != nil {
				t.Error("provider refetched on the tick right after a failure")
			}
			if got := m.list(); !strings.Contains(got, tt.want) {
				t.Errorf("error line %q does not contain %q", got, tt.want)
			}
		})
	}
}

func TestBackoffResets(t *testing.T) {
	boom := fetchedMsg{name: "p", err: errors.New("boom")}
	t.Run("manual refresh", func(t *testing.T) {
		m := newBackoffModel(t)
		for i := 0; i < 3; i++ {
			m.Update(boom)
		}
		m.refreshAll()
		setFetching(m, "p", false)
		m.Update(boom)
		if got := m.list(); !strings.Contains(got, "retry in 30s") {
			t.Errorf("wait not reset by refresh: %q", got)
		}
	})
	t.Run("success", func(t *testing.T) {
		m := newBackoffModel(t)
		for i := 0; i < 3; i++ {
			m.Update(boom)
		}
		m.Update(fetchedMsg{name: "p"})
		m.Update(boom)
		if got := m.list(); !strings.Contains(got, "retry in 30s") {
			t.Errorf("wait not reset by success: %q", got)
		}
	})
}

func TestFetchedCacheWriteRunsInCmd(t *testing.T) {
	t.Run("written by the returned command, not by Update", func(t *testing.T) {
		m := newBackoffModel(t)
		_, cmd := m.Update(fetchedMsg{name: "p", items: mkItems("a", "b")})
		if got := cache.Load("p"); len(got) != 0 {
			t.Fatalf("Update wrote the cache itself: %v", got)
		}
		runAll(cmd)
		if got := cache.Load("p"); len(got) != 2 || got[0].ID != "a" {
			t.Errorf("cache after running the command = %v, want items a, b", got)
		}
	})
	t.Run("a write failure is ignored", func(t *testing.T) {
		m := newBackoffModel(t)
		blocker := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(blocker, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("XDG_CACHE_HOME", blocker) // the cache dir cannot be created below a file
		_, cmd := m.Update(fetchedMsg{name: "p", items: mkItems("a")})
		runAll(cmd)
		if got := m.data["p"]; len(got) != 1 || got[0].ID != "a" {
			t.Errorf("items after a failed write = %v, want item a", got)
		}
	})
}
