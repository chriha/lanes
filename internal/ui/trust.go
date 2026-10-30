package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// trustDialog asks whether the user trusts the project at root. Only typing yes trusts it.
type trustDialog struct {
	root string
	yes  bool
}

func newTrustDialog(root string) trustDialog { return trustDialog{root: root} }

func (m trustDialog) Init() tea.Cmd                       { return nil }
func (m trustDialog) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (m trustDialog) View() string                        { return "" }

// AskTrust runs the trust dialog on stderr and reports whether the user typed yes.
func AskTrust(root string) (bool, error) { return false, nil }
