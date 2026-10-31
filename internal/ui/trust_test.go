package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/trust"
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

func TestReloadChecksTrust(t *testing.T) {
	const before, after = "# a\n", "# b\n"
	tests := []struct {
		name        string
		trustAfter  bool    // the user trusted the changed .lanes.toml (e.g. by restarting lanes)
		msg         tea.Msg // nil = a reload from the tick or ctrl-r
		wantApplied bool
		wantTrusted bool // trust.json trusts the changed file afterwards
	}{
		{"untrusted change is kept out", false, nil, false, false},
		{"trusted change is applied", true, nil, true, true},
		{":edit project trusts and applies it", false, editorDone{project: true}, true, true},
		{":edit of the global file does not trust it", false, editorDone{}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBackoffModel(t) // fresh HOME and XDG dirs
			dir, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			old, _ := os.Getwd()
			defer os.Chdir(old)
			if err := os.Chdir(dir); err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(dir, config.ProjectFile)
			write := func(s string) {
				if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			record := func() {
				hash, err := trust.HashRoot(dir)
				if err != nil {
					t.Fatal(err)
				}
				if err := trust.Trust(dir, hash); err != nil {
					t.Fatal(err)
				}
			}
			write(before)
			record() // lanes started trusted
			write(after)
			if tt.trustAfter {
				record()
			}
			cfg := m.cfg
			if tt.msg == nil {
				m.reload()
			} else {
				m.Update(tt.msg)
			}
			if applied := m.cfg != cfg; applied != tt.wantApplied {
				t.Errorf("applied = %v, want %v (notice %q)", applied, tt.wantApplied, m.notice)
			}
			if !tt.wantApplied && m.notice != ".lanes.toml changed — restart lanes to trust it" {
				t.Errorf("notice = %q, want the restart hint", m.notice)
			}
			hash, _ := trust.HashRoot(dir)
			if ok, err := trust.IsTrusted(dir, hash); err != nil || ok != tt.wantTrusted {
				t.Errorf("trusted = %v (err %v), want %v", ok, err, tt.wantTrusted)
			}
		})
	}
}
