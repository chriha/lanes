package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/trust"
)

// trustDialog asks whether the user trusts the project at root. Only typing yes trusts it; the checkbox, toggled with
// tab, also trusts it whatever its .lanes.toml says later.
type trustDialog struct {
	root   string
	input  textinput.Model
	yes    bool
	always bool // the "always trust this project" checkbox
	done   bool
}

func newTrustDialog(root string) trustDialog {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = "NO"
	in.Width = 3 // without a width textinput shows only the first placeholder rune
	in.Focus()
	return trustDialog{root: root, input: in}
}

func (m trustDialog) Init() tea.Cmd { return textinput.Blink }

func (m trustDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.Type {
		case tea.KeyTab:
			m.always = !m.always
			return m, nil
		case tea.KeyEnter:
			m.yes = strings.EqualFold(strings.TrimSpace(m.input.Value()), "yes")
			m.done = true
			return m, tea.Quit
		case tea.KeyEsc, tea.KeyCtrlC:
			m.done = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m trustDialog) View() string {
	if m.done {
		return "" // leave the terminal clean
	}
	box := " "
	if m.always {
		box = "x"
	}
	return fmt.Sprintf("Do you trust %s?\n\n"+
		"Its .lanes.toml can run shell commands (command providers, actions).\n\n"+
		"Type yes to trust it: [%s]\n"+
		"[%s] always trust this project, even when its .lanes.toml changes\n\n"+
		"enter confirm · tab toggle always · esc quit\n", m.root, m.input.View(), box)
}

// AskTrust runs the trust dialog on stderr (stdout stays for command output) and reports whether the user typed yes,
// and whether they also checked "always trust this project". always is never true without yes.
func AskTrust(root string) (yes, always bool, err error) {
	m, err := tea.NewProgram(newTrustDialog(root), tea.WithOutput(os.Stderr)).Run()
	if err != nil {
		return false, false, err
	}
	d := m.(trustDialog)
	return d.yes, d.yes && d.always, nil
}

// projectHash is the trust root of the cwd and the hash of its .lanes.toml files, as the startup gate computes them.
func projectHash() (root, hash string, err error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	root = trust.Root(wd)
	hash, err = trust.HashRoot(root)
	return root, hash, err
}

// projectTrusted reports whether the project's .lanes.toml files still have the content the user trusted.
func projectTrusted() (bool, error) {
	root, hash, err := projectHash()
	if err != nil {
		return false, err
	}
	return trust.IsTrusted(root, hash)
}

// trustProject records the project's current .lanes.toml content as trusted.
func trustProject() error {
	root, hash, err := projectHash()
	if err != nil {
		return err
	}
	return trust.Trust(root, hash)
}
