package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/trust"
)

// blockProv blocks in Fetch until its context ends, like a provider stuck on a slow API.
type blockProv struct{}

func (blockProv) Fetch(ctx context.Context) ([]item.Item, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// setFetching marks provider name as having a fetch in flight (or not), without starting one.
func setFetching(m *Model, name string, on bool) {
	if m.fetches == nil {
		m.fetches = map[string]*fetchState{}
	}
	if m.fetches[name] == nil {
		m.fetches[name] = &fetchState{}
	}
	m.fetches[name].running = on
}

// startCmd runs cmd in the background, as Bubble Tea does, and delivers its message on the channel.
func startCmd(cmd tea.Cmd) <-chan tea.Msg {
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	return ch
}

// awaitCancelled waits for a blocked fetch to return; it only returns once its context is cancelled.
func awaitCancelled(t *testing.T, ch <-chan tea.Msg) fetchedMsg {
	t.Helper()
	select {
	case msg := <-ch:
		fm, ok := msg.(fetchedMsg)
		if !ok || !errors.Is(fm.err, context.Canceled) {
			t.Fatalf("fetch returned %#v, want a fetchedMsg with context.Canceled", msg)
		}
		return fm
	case <-time.After(5 * time.Second):
		t.Fatal("the running fetch was not cancelled")
	}
	return fetchedMsg{}
}

func TestFetchStateTransitions(t *testing.T) {
	tests := []struct {
		name         string
		land         *fetchedMsg // landed after the fetch started; nil: nothing lands
		wantFetching bool
		wantLast     bool // the duration of the fetch was recorded
	}{
		{"started fetch is in flight", nil, true, false},
		{"partial result keeps it in flight", &fetchedMsg{partial: true, items: mkItems("a")}, true, false},
		{"full result ends it and records the duration", &fetchedMsg{items: mkItems("a")}, false, true},
		{"failed fetch ends it and records the duration", &fetchedMsg{err: errors.New("boom")}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBackoffModel(t)
			m.providers["p"] = &fakeProv{}
			if m.fetch("p") == nil {
				t.Fatal("fetch of an idle provider returned no command")
			}
			if m.fetch("p") != nil {
				t.Error("a second fetch started while one is in flight")
			}
			st := m.fetches["p"]
			st.started = time.Now().Add(-3 * time.Second)
			if tt.land != nil {
				msg := *tt.land
				msg.name, msg.gen = "p", st.gen
				m.Update(msg)
			}
			if got := m.fetching("p"); got != tt.wantFetching {
				t.Errorf("fetching = %v, want %v", got, tt.wantFetching)
			}
			if got := st.last >= 3*time.Second; got != tt.wantLast {
				t.Errorf("last = %v, want recorded = %v", st.last, tt.wantLast)
			}
		})
	}
}

func TestCancelledFetchResultIsDiscarded(t *testing.T) {
	tests := []struct {
		name  string
		stale fetchedMsg // what the cancelled fetch delivers (name and gen are filled in)
	}{
		{"full result", fetchedMsg{items: mkItems("old")}},
		{"partial result", fetchedMsg{items: mkItems("old"), partial: true}},
		{"error", fetchedMsg{err: errors.New("boom")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBackoffModel(t)
			m.providers["p"] = blockProv{}
			m.data["p"] = mkItems("a")
			ch := startCmd(m.fetch("p"))
			m.cancelFetches()
			cancelled := awaitCancelled(t, ch)
			if m.fetching("p") {
				t.Error("provider still fetching after its fetch was cancelled")
			}
			m.providers["p"] = &fakeProv{}
			if m.fetch("p") == nil {
				t.Fatal("no new fetch after the cancelled one")
			}
			stale := tt.stale
			stale.name, stale.gen = "p", cancelled.gen
			m.Update(stale)
			if got := itemIDs(m.data["p"]); !reflect.DeepEqual(got, []string{"a"}) {
				t.Errorf("data = %v, want the stale result dropped", got)
			}
			if m.provErr["p"] != nil || m.fails["p"] != 0 || !m.fetchedAt["p"].IsZero() {
				t.Errorf("stale result counted: provErr=%v fails=%d fetchedAt=%v", m.provErr["p"], m.fails["p"], m.fetchedAt["p"])
			}
			if !m.fetching("p") {
				t.Error("stale result ended the fetch that replaced it")
			}
		})
	}
}

// trustedProject makes a trusted project with a .lanes.toml, chdirs into it and writes the global config.
func trustedProject(t *testing.T, cfg string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, config.ProjectFile), []byte("# project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash, err := trust.HashRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := trust.Trust(dir, hash); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(app.ConfigDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app.ConfigDir(), "config.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(old) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
}

func TestReloadCancelsRunningFetchAndRefetches(t *testing.T) {
	m := newBackoffModel(t)
	trustedProject(t, "[provider.p]\ntype = \"command\"\ncommand = \"true\"\n[[view]]\nname = \"v\"\nprovider = \"p\"\n")
	m.providers["p"] = blockProv{}
	m.data["p"] = mkItems("a")
	ch := startCmd(m.fetch("p"))
	m.refetchAfter["p"] = true // a refresh run ended while the old fetch was running

	cmd := landReload(t, m, m.reload())
	old := awaitCancelled(t, ch)
	if cmd == nil || !m.fetching("p") {
		t.Fatalf("reload did not start a new fetch at once: cmd=%v fetching=%v", cmd != nil, m.fetching("p"))
	}
	if m.refetchAfter["p"] {
		t.Error("the new fetch starts after the run; it needs no follow-up fetch")
	}
	m.Update(old)
	if !m.fetching("p") || m.provErr["p"] != nil {
		t.Errorf("the cancelled fetch's result was applied: fetching=%v provErr=%v", m.fetching("p"), m.provErr["p"])
	}
	m.Update(fetchedMsg{name: "p", gen: m.fetches["p"].gen, items: mkItems("new")})
	if got := itemIDs(m.data["p"]); !reflect.DeepEqual(got, []string{"new"}) || m.fetching("p") {
		t.Errorf("new fetch result: data=%v fetching=%v, want [new] and done", got, m.fetching("p"))
	}
}

func TestCloseCancelsRunningFetches(t *testing.T) {
	m := newBackoffModel(t)
	m.providers["p"] = blockProv{}
	ch := startCmd(m.fetch("p"))
	m.Close()
	awaitCancelled(t, ch)
}

func TestFetchStatus(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		setup func(m *Model)
		want  string
	}{
		{"cached: nothing fetched in this run", func(*Model) {}, "p cached"},
		{"fetching: spinner and elapsed", func(m *Model) {
			m.fetches = map[string]*fetchState{"p": {running: true, started: now.Add(-4 * time.Second)}}
		}, "p ⠼ fetching 4s"},
		{"fetching again: also the last duration", func(m *Model) {
			m.fetches = map[string]*fetchState{"p": {running: true, started: now.Add(-1 * time.Second), last: 12340 * time.Millisecond}}
			m.fetchedAt["p"] = now.Add(-time.Minute)
		}, "p ⠙ fetching 1s · last 12.3s"},
		{"failed: phase and last duration", func(m *Model) {
			m.fetches = map[string]*fetchState{"p": {last: 2 * time.Second}}
			m.provErr["p"] = errors.New("boom")
		}, "p ✗ failed · 2s"},
		{"idle: age and last duration", func(m *Model) {
			m.fetches = map[string]*fetchState{"p": {last: 250 * time.Millisecond}}
			m.fetchedAt["p"] = now.Add(-12 * time.Second)
		}, "p updated 12s ago · 250ms"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBackoffModel(t)
			tt.setup(m)
			if got := m.fetchStatus("p", now); got != tt.want {
				t.Errorf("fetchStatus = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFetchStatusAdvancesEachTick(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	m := newBackoffModel(t)
	m.fetches = map[string]*fetchState{"p": {running: true, started: now}}
	a, b := m.fetchStatus("p", now.Add(time.Second)), m.fetchStatus("p", now.Add(2*time.Second))
	if a == b || !strings.Contains(b, "fetching 2s") {
		t.Errorf("status one tick apart: %q then %q, want it to advance to 2s", a, b)
	}
}

func newTwoProviderModel(t *testing.T) *Model {
	t.Helper()
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "HOME"} {
		t.Setenv(k, t.TempDir())
	}
	m := New(&config.Config{
		Provider: map[string]config.Provider{
			"gitlab": {Type: "command", Command: "true"},
			"linear": {Type: "command", Command: "true"},
		},
		View: []config.View{{Name: "v", Provider: "gitlab", Filter: "true"}},
	})
	m.width, m.height = 160, 20
	return m
}

func TestHeaderShowsEveryProviderStatus(t *testing.T) {
	m := newTwoProviderModel(t)
	setFetching(m, "linear", true)
	m.fetches["linear"].started = time.Now()
	m.fetchedAt["gitlab"] = time.Now()
	h := ansi.Strip(m.header())
	for _, want := range []string{"gitlab updated", "linear ⠋ fetching 0s"} {
		if !strings.Contains(h, want) {
			t.Errorf("header %q lacks %q", h, want)
		}
	}
	if strings.Contains(h, "refreshing") {
		t.Errorf("header %q still shows the bare refresh marker", h)
	}
}

func TestNarrowHeaderShowsOnlyTheViewsProvider(t *testing.T) {
	m := newTwoProviderModel(t)
	m.width = 40
	h := ansi.Strip(m.header())
	if !strings.Contains(h, "gitlab cached") || strings.Contains(h, "linear") {
		t.Errorf("narrow header %q, want only the view's provider status", h)
	}
}

func TestEmptyListShowsFetchStatus(t *testing.T) {
	tests := []struct {
		name  string
		setup func(m *Model)
		want  []string
		not   []string
	}{
		{"fetching: status lines instead of loading…", func(m *Model) {
			setFetching(m, "gitlab", true)
			m.fetches["gitlab"].started = time.Now()
		}, []string{"gitlab ⠋ fetching 0s", "linear cached"}, []string{"loading…"}},
		{"idle and empty: nothing here", func(m *Model) {
			m.fetchedAt["gitlab"] = time.Now()
		}, []string{"nothing here"}, []string{"fetching"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTwoProviderModel(t)
			tt.setup(m)
			l := ansi.Strip(m.list())
			for _, w := range tt.want {
				if !strings.Contains(l, w) {
					t.Errorf("list %q lacks %q", l, w)
				}
			}
			for _, n := range tt.not {
				if strings.Contains(l, n) {
					t.Errorf("list %q shows %q", l, n)
				}
			}
		})
	}
}
