package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/item"
)

func TestApplyConfigClearsStaleProviderError(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := func(typ string) *config.Config {
		return &config.Config{
			Provider: map[string]config.Provider{"p": {Type: typ, Command: "true"}},
			View:     []config.View{{Name: "v", Provider: "p"}},
		}
	}
	m := &Model{
		ov:        &config.Overrides{Filters: map[string]string{}},
		data:      map[string][]item.Item{},
		fetchedAt: map[string]time.Time{},
		provErr:   map[string]error{},
		marks:     map[string]bool{},
	}
	m.applyConfig(cfg("bogus"))
	if m.provErr["p"] == nil {
		t.Fatal("a failed build should record provErr")
	}
	m.applyConfig(cfg("command"))
	setFetching(m, "p", true) // the refetch the reload triggers
	m.fetches["p"].started = time.Now()
	if m.provErr["p"] != nil {
		t.Errorf("provErr = %v, want cleared once the provider builds", m.provErr["p"])
	}
	if out := m.list(); strings.Contains(out, "bogus") || !strings.Contains(ansi.Strip(out), "p [") || !strings.Contains(ansi.Strip(out), "] fetching") {
		t.Errorf("list should show the fetch status, got %q", out)
	}
}
