package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/config"
)

// landReload runs the reload command cmd as Bubble Tea would and hands its message to the model.
func landReload(t *testing.T, m *Model, cmd tea.Cmd) tea.Cmd {
	t.Helper()
	if cmd == nil {
		t.Fatal("reload returned no command")
	}
	_, next := m.Update(cmd())
	return next
}

// writeGlobalConfig replaces the global config.toml.
func writeGlobalConfig(t *testing.T, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(app.ConfigDir(), "config.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const twoProviders = "[provider.p]\ntype = \"command\"\ncommand = \"true\"\n" +
	"[provider.q]\ntype = \"command\"\ncommand = \"true\"\n" +
	"[[view]]\nname = \"v\"\nprovider = \"p\"\n[[view]]\nname = \"w\"\nprovider = \"q\"\n"

func TestReloadAppliesThroughMessage(t *testing.T) {
	tests := []struct {
		name        string
		cfg         string
		wantApplied bool
		wantNotice  string // prefix of the notice once the reload landed
	}{
		{"valid config", twoProviders, true, "config reloaded"},
		{"broken config", "not [valid", false, "config: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBackoffModel(t)
			trustedProject(t, tt.cfg)
			old := m.cfg
			m.say(lvInfo, "")

			cmd := m.reload()
			if m.cfg != old || len(m.providers) != 1 || m.notice != "" {
				t.Fatalf("reload applied before its message landed: cfg changed=%v providers=%d notice=%q",
					m.cfg != old, len(m.providers), m.notice)
			}
			press(m, "j") // keys keep working while the reload runs
			if m.cfg != old {
				t.Fatal("a key press applied the pending reload")
			}

			landReload(t, m, cmd)
			if applied := m.cfg != old; applied != tt.wantApplied {
				t.Errorf("applied = %v, want %v (notice %q)", applied, tt.wantApplied, m.notice)
			}
			if _, ok := m.providers["q"]; ok != tt.wantApplied {
				t.Errorf("provider q built = %v, want %v", ok, tt.wantApplied)
			}
			if !strings.HasPrefix(m.notice, tt.wantNotice) {
				t.Errorf("notice = %q, want prefix %q", m.notice, tt.wantNotice)
			}
		})
	}
}

func TestStaleReloadIsDropped(t *testing.T) {
	m := newBackoffModel(t)
	trustedProject(t, twoProviders)
	first := m.reload()
	firstMsg := first() // reads the two-provider config
	writeGlobalConfig(t, "[provider.p]\ntype = \"command\"\ncommand = \"true\"\n[[view]]\nname = \"only\"\nprovider = \"p\"\n")
	second := m.reload()

	landReload(t, m, second)
	m.Update(firstMsg) // the older reload lands last
	if len(m.views) != 1 || m.views[0].cfg.Name != "only" {
		names := []string{}
		for _, v := range m.views {
			names = append(names, v.cfg.Name)
		}
		t.Errorf("views = %v, want [only]: the older reload overwrote the newer one", names)
	}
}

func TestTickStartsOneReloadPerChange(t *testing.T) {
	m := newBackoffModel(t)
	trustedProject(t, twoProviders)
	// the config is newer than the model's: the first tick starts a reload, the next ones must not repeat it
	m.cfgMod = time.Time{}
	before := m.cfg
	m.onTick()
	m.onTick()
	if m.cfg != before {
		t.Error("the tick applied the reload on the UI goroutine")
	}
	if !config.Mtime().Equal(m.cfgMod) {
		t.Errorf("cfgMod = %v, want the config's mtime %v once the reload started", m.cfgMod, config.Mtime())
	}
}

func TestBlockingProviderLeavesUIResponsive(t *testing.T) {
	m := newBackoffModel(t)
	m.providers["p"] = blockProv{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Init() // the fetch it starts runs in Bubble Tea's goroutine, not here
		m.Update(tickMsg(time.Now()))
		press(m, "j")
		m.View()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Init, tick, key press or View blocked on a provider whose fetch never returns")
	}
	m.Close()
}

func TestNewSpawnsNoSubprocessForAuto(t *testing.T) {
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "HOME"} {
		t.Setenv(k, t.TempDir())
	}
	t.Setenv("PATH", t.TempDir()) // no git: building auto would fail if it ran git remote get-url
	m := New(&config.Config{
		Provider: map[string]config.Provider{"p": {Type: "auto"}},
		View:     []config.View{{Name: "v", Provider: "p", Filter: "true"}},
	})
	if err := m.provErr["p"]; err != nil {
		t.Errorf("New read the git remote for the auto provider: %v", err)
	}
	if m.providers["p"] == nil {
		t.Error("auto provider not built")
	}
}
