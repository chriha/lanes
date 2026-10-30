package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestTrustDialog(t *testing.T) {
	enter := tea.KeyMsg{Type: tea.KeyEnter}
	tests := []struct {
		name  string
		typed string
		last  tea.KeyMsg
		want  bool
	}{
		{"yes", "yes", enter, true},
		{"YES", "YES", enter, true},
		{"padded yes", " yes ", enter, true},
		{"y", "y", enter, false},
		{"empty", "", enter, false},
		{"no", "no", enter, false},
		{"esc after yes", "yes", tea.KeyMsg{Type: tea.KeyEsc}, false},
		{"ctrl+c after yes", "yes", tea.KeyMsg{Type: tea.KeyCtrlC}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m tea.Model = newTrustDialog("/p")
			for _, r := range tt.typed {
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			}
			m, cmd := m.Update(tt.last)
			if cmd == nil {
				t.Fatal("dialog did not quit")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatal("dialog did not quit")
			}
			if got := m.(trustDialog).yes; got != tt.want {
				t.Errorf("trusted = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTrustDialogView(t *testing.T) {
	v := ansi.Strip(newTrustDialog("/some/project").View())
	for _, want := range []string{"/some/project", ".lanes.toml", "shell commands", "Type yes to trust it", "NO", "enter confirm · esc quit"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
}
