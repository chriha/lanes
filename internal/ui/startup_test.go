package ui

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/provider"
)

// blockingProv never answers until release is closed. quicker makes it a GitLab-like provider with a quick pass.
type blockingProv struct {
	release chan struct{}
	wg      *sync.WaitGroup
}

func (b *blockingProv) Fetch(context.Context) ([]item.Item, error) {
	defer b.wg.Done()
	<-b.release
	return mkItems("1"), nil
}

type blockingQuicker struct{ blockingProv }

func (b *blockingQuicker) Quick(context.Context) ([]item.Item, error) {
	defer b.wg.Done()
	<-b.release
	return mkItems("1"), nil
}

// frames collects what the program draws and wakes a waiter on every write.
type frames struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	wake chan struct{}
}

func (f *frames) Write(p []byte) (int, error) {
	f.mu.Lock()
	f.buf.Write(p)
	f.mu.Unlock()
	select {
	case f.wake <- struct{}{}:
	default:
	}
	return len(p), nil
}

func (f *frames) has(s string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Contains(f.buf.String(), s)
}

// waitFor blocks until the output contains s, failing the test after a generous deadline.
func (f *frames) waitFor(t *testing.T, s, what string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for !f.has(s) {
		select {
		case <-f.wake:
		case <-deadline:
			t.Fatalf("timed out waiting for %s (%q not drawn)", what, s)
		}
	}
}

// Both providers hang (a GitLab-like one with a quick pass, a Linear-like one without): the first frame is
// drawn and keys are handled and redrawn while they fetch, and the program exits cleanly once they are released.
func TestStartDrawsAndHandlesKeysWhileProvidersBlock(t *testing.T) {
	m, _ := busyModel(t) // isolated XDG dirs, no tmux polling
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(3) // gitlab: Quick and Fetch, linear: Fetch
	m.providers = map[string]provider.Provider{
		"gitlab": &blockingQuicker{blockingProv{release, &wg}},
		"linear": &blockingProv{release, &wg},
	}
	m.cfg.Provider = map[string]config.Provider{"gitlab": {Type: "gitlab"}, "linear": {Type: "linear"}}
	m.data = map[string][]item.Item{}
	f := m.views[0].filter
	m.views = []*view{
		{cfg: config.View{Name: "gl", Provider: "gitlab"}, filter: f},
		{cfg: config.View{Name: "li", Provider: "linear"}, filter: f},
	}
	m.rebuild()

	out := &frames{wake: make(chan struct{}, 1)}
	prog := tea.NewProgram(m, tea.WithInput(nil), tea.WithOutput(out), tea.WithoutSignals())
	t.Cleanup(func() { // a failed wait must not leave the fakes and the program running
		select {
		case <-release:
		default:
			close(release)
		}
		prog.Kill()
	})
	done := make(chan error, 1)
	go func() { _, err := prog.Run(); done <- err }()

	out.waitFor(t, "fetching", "the first frame") // the footer of a view whose provider is still loading
	prog.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	out.waitFor(t, "move down / up", "the help screen after a key press")

	close(release)
	fetched := make(chan struct{})
	go func() { wg.Wait(); close(fetched) }()
	select {
	case <-fetched:
	case <-time.After(10 * time.Second):
		t.Fatal("a fetch did not return after release")
	}
	prog.Quit()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("program exited with %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("program did not exit")
	}
}
