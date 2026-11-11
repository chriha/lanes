package ui

import (
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/trust"
)

// trustCheckedMsg is the startup trust check's result (trust.Check on the root), computed in a tea.Cmd so the first
// frame does not wait for the walk over the project tree.
type trustCheckedMsg struct {
	trusted bool
	hash    string // the root's .lanes.toml hash; "" for an always-trusted root
	err     error
}

// Startup is the program's first model, the startup trust gate: it draws a "checking project trust" frame, checks
// root's trust off the UI goroutine, shows the trust dialog when the project is untrusted, and only once it is trusted
// calls load (initApp, config load, New) and hands every message to the loaded app.
type Startup struct {
	root    string
	load    func() (*Model, error) // runs only after the trusted result or a yes in the dialog
	checked bool                   // the trust check has answered; false: View draws the checking frame
	hash    string                 // the root's hash the check computed, recorded when the dialog trusts it
	dialog  *trustDialog           // the trust dialog for an untrusted result, nil otherwise
	size    *tea.WindowSizeMsg     // the last window size, handed to the app once it loads
	app     *Model                 // the loaded app, nil until trusted
	err     error                  // what ended the program: a failed check, trust write or load, or a declined dialog
}

// NewStartup returns the startup gate for the project at root; load builds the app once the project is trusted.
func NewStartup(root string, load func() (*Model, error)) *Startup {
	return &Startup{root: root, load: load}
}

// Init starts the trust check, which walks the root's tree for its .lanes.toml files.
func (s *Startup) Init() tea.Cmd {
	root := s.root
	return func() tea.Msg {
		trusted, hash, err := trust.Check(root)
		return trustCheckedMsg{trusted: trusted, hash: hash, err: err}
	}
}

// Update handles the trust result and the dialog, then forwards every message to the app.
func (s *Startup) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if s.app != nil {
		next, cmd := s.app.Update(msg)
		if app, ok := next.(*Model); ok {
			s.app = app
		}
		return s, cmd
	}
	if s.err != nil {
		return s, nil // quitting: a late trust result must not load the app
	}
	switch msg := msg.(type) {
	case trustCheckedMsg:
		s.checked, s.hash = true, msg.hash
		if msg.err != nil {
			return s.quit(msg.err)
		}
		if msg.trusted {
			return s.start()
		}
		d := newTrustDialog(s.root)
		s.dialog = &d
		return s, d.Init()
	case tea.WindowSizeMsg:
		s.size = &msg
	case tea.KeyMsg:
		if s.dialog == nil && msg.Type == tea.KeyCtrlC {
			return s.quit(errors.New("interrupted"))
		}
	}
	if s.dialog != nil {
		return s.answer(msg)
	}
	return s, nil
}

// answer passes msg to the trust dialog; once it is done, a yes records the trust and loads the app.
func (s *Startup) answer(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := s.dialog.Update(msg)
	d := next.(trustDialog)
	s.dialog = &d
	if !d.done {
		return s, cmd // the dialog's own tea.Quit on done is dropped: the program goes on to the app
	}
	s.dialog = nil
	if !d.yes {
		return s.quit(errors.New("not trusted"))
	}
	var err error
	if d.always {
		err = trust.TrustAlways(s.root)
	} else {
		err = trust.Trust(s.root, s.hash)
	}
	if err != nil {
		return s.quit(err)
	}
	return s.start()
}

// start loads the app of the trusted project and hands it the window size it missed.
func (s *Startup) start() (tea.Model, tea.Cmd) {
	app, err := s.load()
	if err != nil {
		return s.quit(err)
	}
	s.app = app
	cmd := app.Init()
	if s.size != nil {
		_, sized := app.Update(*s.size)
		cmd = tea.Batch(cmd, sized)
	}
	return s, cmd
}

func (s *Startup) quit(err error) (tea.Model, tea.Cmd) {
	s.err = err
	return s, tea.Quit
}

// View draws the checking frame, the trust dialog or the app.
func (s *Startup) View() string {
	switch {
	case s.app != nil:
		return s.app.View()
	case s.dialog != nil:
		return s.dialog.View()
	case !s.checked:
		return fmt.Sprintf("checking whether %s is trusted…\n", s.root)
	}
	return ""
}

// Err is what ended the program without running the app; nil when the app ran and quit.
func (s *Startup) Err() error { return s.err }

// Close stops the loaded app's fetches; a no-op when the app never loaded.
func (s *Startup) Close() {
	if s.app != nil {
		s.app.Close()
	}
}
