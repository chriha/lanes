package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/session"
)

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestQuit(t *testing.T) {
	tests := []struct {
		name   string
		keys   []string
		quits  bool
		prompt string // expected confirm prompt after the keys, "" if none
	}{
		{"q asks", []string{"q"}, false, "quit lanes? (2 sessions keep running) [y/N]"},
		{"y quits", []string{"q", "y"}, true, ""},
		{"enter quits", []string{"q", "enter"}, true, ""},
		{"n cancels", []string{"q", "n"}, false, ""},
		{"esc cancels", []string{"q", "esc"}, false, ""},
		{"other key cancels", []string{"q", "j"}, false, ""},
		{"ctrl+c is immediate", []string{"ctrl+c"}, true, ""},
		{":q is immediate", []string{"cmd:q"}, true, ""},
		{":quit is immediate", []string{"cmd:quit"}, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := sessModel(t, session.Info{Name: "a"}, session.Info{Name: "b"}, session.Info{Name: "c", Dead: true})
			var cmd tea.Cmd
			for _, k := range tt.keys {
				if line, ok := strings.CutPrefix(k, "cmd:"); ok {
					_, cmd = m.runCommand(line)
					continue
				}
				msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
				switch k {
				case "esc":
					msg = tea.KeyMsg{Type: tea.KeyEsc}
				case "enter":
					msg = tea.KeyMsg{Type: tea.KeyEnter}
				case "ctrl+c":
					msg = tea.KeyMsg{Type: tea.KeyCtrlC}
				}
				_, cmd = m.key(msg)
			}
			if got := isQuit(cmd); got != tt.quits {
				t.Errorf("quit = %v, want %v", got, tt.quits)
			}
			got := ""
			if m.mode == modeConfirm {
				got = m.confirm.prompt
			}
			if got != tt.prompt {
				t.Errorf("prompt = %q, want %q", got, tt.prompt)
			}
			if tt.prompt != "" && !strings.Contains(m.View(), "quit lanes?") {
				t.Error("prompt not rendered")
			}
		})
	}
}

func TestQuitPromptCount(t *testing.T) {
	tests := []struct {
		name  string
		infos []session.Info
		want  string
	}{
		{"none", nil, "quit lanes? (0 sessions keep running) [y/N]"},
		{"one", []session.Info{{Name: "a"}}, "quit lanes? (1 session keeps running) [y/N]"},
		{"dead not counted", []session.Info{{Name: "a"}, {Name: "b", Dead: true}}, "quit lanes? (1 session keeps running) [y/N]"},
		{"two", []session.Info{{Name: "a"}, {Name: "b"}}, "quit lanes? (2 sessions keep running) [y/N]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := sessModel(t, tt.infos...)
			m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
			if m.confirm.prompt != tt.want {
				t.Errorf("prompt = %q, want %q", m.confirm.prompt, tt.want)
			}
		})
	}
}
