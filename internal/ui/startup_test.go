package ui

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/provider"
	"github.com/chriha/lanes/internal/trust"
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

// startupRoot makes a project root with a .lanes.toml in an isolated state dir and returns it with its hash.
func startupRoot(t *testing.T) (root, hash string) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root = t.TempDir()
	if err := os.WriteFile(filepath.Join(root, config.ProjectFile), []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash, err := trust.HashRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	return root, hash
}

// countingLoad returns a load step that counts its calls and returns app (or err).
func countingLoad(app *Model, err error, n *int) func() (*Model, error) {
	return func() (*Model, error) {
		*n++
		if err != nil {
			return nil, err
		}
		return app, nil
	}
}

// checkTrust runs the startup gate's Init and returns the trust message its command delivers.
func checkTrust(t *testing.T, s *Startup) tea.Msg {
	t.Helper()
	cmd := s.Init()
	if cmd == nil {
		t.Fatal("Init starts no trust check")
	}
	return cmd()
}

func TestStartupGate(t *testing.T) {
	tests := []struct {
		name       string
		stored     string // trust.json for the root: "" none, "hash", "always", "old" (a different hash), "corrupt"
		wantDialog bool
		wantLoads  int
		wantErr    string // substring; "" = no error
	}{
		{"no entry asks", "", true, 0, ""},
		{"changed file asks", "old", true, 0, ""},
		{"same hash loads", "hash", false, 1, ""},
		{"always loads", "always", false, 1, ""},
		{"corrupt store quits", "corrupt", false, 0, "trust.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, _ := busyModel(t)
			root, hash := startupRoot(t)
			var err error
			switch tt.stored {
			case "hash":
				err = trust.Trust(root, hash)
			case "always":
				err = trust.TrustAlways(root)
			case "old":
				err = trust.Trust(root, "old")
			case "corrupt":
				store := filepath.Join(os.Getenv("XDG_STATE_HOME"), "lanes", "trust.json")
				if err = os.MkdirAll(filepath.Dir(store), 0o755); err == nil {
					err = os.WriteFile(store, []byte("{broken"), 0o600)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			loads := 0
			s := NewStartup(root, countingLoad(app, nil, &loads))
			msg := checkTrust(t, s)
			if !strings.Contains(s.View(), "checking") || loads != 0 {
				t.Fatalf("before the check answers: view %q, loads %d; want the checking frame and no load", s.View(), loads)
			}
			_, cmd := s.Update(msg)
			if got := strings.Contains(s.View(), "Do you trust"); got != tt.wantDialog {
				t.Errorf("dialog shown = %v, want %v (view %q)", got, tt.wantDialog, s.View())
			}
			if loads != tt.wantLoads {
				t.Errorf("loads = %d, want %d", loads, tt.wantLoads)
			}
			if tt.wantErr == "" && s.Err() != nil || tt.wantErr != "" && (s.Err() == nil || !strings.Contains(s.Err().Error(), tt.wantErr)) {
				t.Errorf("Err = %v, want %q", s.Err(), tt.wantErr)
			}
			if got := isQuit(cmd); got != (tt.wantErr != "") {
				t.Errorf("quits = %v, want %v", got, tt.wantErr != "")
			}
		})
	}
}

func TestStartupDialog(t *testing.T) {
	enter := tea.KeyMsg{Type: tea.KeyEnter}
	tests := []struct {
		name       string
		tab        bool // check "always trust"
		typed      string
		last       tea.KeyMsg
		wantLoads  int
		wantHash   bool // trust.json trusts the current hash afterwards
		wantAlways bool // trust.json trusts the root whatever the hash
	}{
		{"yes stores the hash and loads", false, "yes", enter, 1, true, false},
		{"checked yes stores always and loads", true, "yes", enter, 1, true, true},
		{"no quits", false, "no", enter, 0, false, false},
		{"checked no quits", true, "no", enter, 0, false, false},
		{"esc quits", false, "yes", tea.KeyMsg{Type: tea.KeyEsc}, 0, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, _ := busyModel(t)
			root, hash := startupRoot(t)
			loads := 0
			s := NewStartup(root, countingLoad(app, nil, &loads))
			s.Update(checkTrust(t, s))
			if tt.tab {
				s.Update(tea.KeyMsg{Type: tea.KeyTab})
			}
			for _, r := range tt.typed {
				s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			}
			_, cmd := s.Update(tt.last)
			if loads != tt.wantLoads {
				t.Errorf("loads = %d, want %d", loads, tt.wantLoads)
			}
			declined := tt.wantLoads == 0
			if got := isQuit(cmd); got != declined {
				t.Errorf("quits = %v, want %v", got, declined)
			}
			if declined != (s.Err() != nil) || declined && !strings.Contains(s.Err().Error(), "not trusted") {
				t.Errorf("Err = %v, want not trusted: %v", s.Err(), declined)
			}
			if ok, err := trust.IsTrusted(root, hash); err != nil || ok != tt.wantHash {
				t.Errorf("hash trusted = %v (err %v), want %v", ok, err, tt.wantHash)
			}
			if ok, err := trust.IsTrusted(root, "other"); err != nil || ok != tt.wantAlways {
				t.Errorf("always trusted = %v (err %v), want %v", ok, err, tt.wantAlways)
			}
		})
	}
}

// A failed load (a broken config) ends the program with its error instead of running the app.
func TestStartupLoadError(t *testing.T) {
	root, _ := startupRoot(t)
	if err := trust.TrustAlways(root); err != nil {
		t.Fatal(err)
	}
	loads := 0
	s := NewStartup(root, countingLoad(nil, errors.New("bad config"), &loads))
	_, cmd := s.Update(checkTrust(t, s))
	if loads != 1 || !isQuit(cmd) || s.Err() == nil || !strings.Contains(s.Err().Error(), "bad config") {
		t.Errorf("loads %d, quits %v, Err %v; want one load, quit and the load error", loads, isQuit(cmd), s.Err())
	}
}

// ctrl+c while the walk runs quits without loading, and a late trusted answer starts nothing.
func TestStartupCtrlCWhileChecking(t *testing.T) {
	app, _ := busyModel(t)
	root, _ := startupRoot(t)
	if err := trust.TrustAlways(root); err != nil {
		t.Fatal(err)
	}
	loads := 0
	s := NewStartup(root, countingLoad(app, nil, &loads))
	msg := checkTrust(t, s)
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !isQuit(cmd) || s.Err() == nil {
		t.Errorf("quits %v, Err %v; want quit with an error", isQuit(cmd), s.Err())
	}
	s.Update(msg)
	if loads != 0 {
		t.Errorf("loads = %d after ctrl+c, want 0", loads)
	}
}

// The window size arrives before the app exists; the app still gets it, and later messages go to the app.
func TestStartupHandsOverToApp(t *testing.T) {
	app, _ := busyModel(t)
	root, _ := startupRoot(t)
	if err := trust.TrustAlways(root); err != nil {
		t.Fatal(err)
	}
	loads := 0
	s := NewStartup(root, countingLoad(app, nil, &loads))
	msg := checkTrust(t, s)
	s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	s.Update(msg)
	if app.width != 80 || app.height != 24 {
		t.Fatalf("app size = %dx%d, want 80x24", app.width, app.height)
	}
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if v := s.View(); v != app.View() || !strings.Contains(v, "move down / up") {
		t.Errorf("view = %q, want the app's help screen", v)
	}
}
