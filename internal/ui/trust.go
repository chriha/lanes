package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// trustDialog asks whether the user trusts the project at root. Only typing yes trusts it.
type trustDialog struct {
	root  string
	input textinput.Model
	yes   bool
	done  bool
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
	return fmt.Sprintf("Do you trust %s?\n\n"+
		"Its .lanes.toml can run shell commands (command providers, actions).\n\n"+
		"Type yes to trust it: [%s]\n\n"+
		"enter confirm · esc quit\n", m.root, m.input.View())
}

// AskTrust runs the trust dialog on stderr (stdout stays for command output) and reports whether the user typed yes.
func AskTrust(root string) (bool, error) {
	m, err := tea.NewProgram(newTrustDialog(root), tea.WithOutput(os.Stderr)).Run()
	if err != nil {
		return false, err
	}
	return m.(trustDialog).yes, nil
}
