package ui

import tea "github.com/charmbracelet/bubbletea"

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
	trusted bool                   // the check's answer
	hash    string                 // the root's hash the check computed, recorded when the dialog trusts it
	dialog  *trustDialog           // the trust dialog for an untrusted result, nil otherwise
	app     *Model                 // the loaded app, nil until trusted
	err     error                  // what ended the program: a failed check, trust write or load, or a declined dialog
}

// NewStartup returns the startup gate for the project at root; load builds the app once the project is trusted.
func NewStartup(root string, load func() (*Model, error)) *Startup {
	return &Startup{root: root, load: load}
}

// Init starts the trust check. Stub: checks nothing yet.
func (s *Startup) Init() tea.Cmd { return nil }

// Update handles the trust result and the dialog, then forwards to the app. Stub: only keeps the trust result.
func (s *Startup) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if r, ok := msg.(trustCheckedMsg); ok {
		s.checked, s.trusted, s.hash, s.err = true, r.trusted, r.hash, r.err
	}
	return s, nil
}

// View draws the checking frame, the trust dialog or the app. Stub: draws only the dialog, which nothing opens yet.
func (s *Startup) View() string {
	if s.dialog != nil {
		return s.dialog.View()
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
