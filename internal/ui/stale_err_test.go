package ui

import (
	"strings"
	"testing"
	"time"

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
		loading:   map[string]bool{},
		provErr:   map[string]error{},
		marks:     map[string]bool{},
	}
	m.applyConfig(cfg("bogus"))
	if m.provErr["p"] == nil {
		t.Fatal("a failed build should record provErr")
	}
	m.applyConfig(cfg("command"))
	m.loading["p"] = true // the refetch the reload triggers
	if m.provErr["p"] != nil {
		t.Errorf("provErr = %v, want cleared once the provider builds", m.provErr["p"])
	}
	if out := m.list(); strings.Contains(out, "bogus") || !strings.Contains(out, "loading…") {
		t.Errorf("list should show loading…, got %q", out)
	}
}
