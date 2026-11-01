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
		name       string
		tabs       int // tab presses before typing
		typed      string
		last       tea.KeyMsg
		want       bool
		wantAlways bool // the checkbox is checked; the gate uses it only with yes
	}{
		{"yes", 0, "yes", enter, true, false},
		{"YES", 0, "YES", enter, true, false},
		{"padded yes", 0, " yes ", enter, true, false},
		{"y", 0, "y", enter, false, false},
		{"empty", 0, "", enter, false, false},
		{"no", 0, "no", enter, false, false},
		{"esc after yes", 0, "yes", tea.KeyMsg{Type: tea.KeyEsc}, false, false},
		{"ctrl+c after yes", 0, "yes", tea.KeyMsg{Type: tea.KeyCtrlC}, false, false},
		{"checked yes", 1, "yes", enter, true, true},
		{"checked YES", 1, "YES", enter, true, true},
		{"checked twice is unchecked", 2, "yes", enter, true, false},
		{"checked thrice is checked", 3, "yes", enter, true, true},
		{"checked y", 1, "y", enter, false, true},
		{"checked empty", 1, "", enter, false, true},
		{"checked no", 1, "no", enter, false, true},
		{"checked esc after yes", 1, "yes", tea.KeyMsg{Type: tea.KeyEsc}, false, true},
		{"checked ctrl+c after yes", 1, "yes", tea.KeyMsg{Type: tea.KeyCtrlC}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m tea.Model = newTrustDialog("/p")
			for range tt.tabs {
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
			}
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
			d := m.(trustDialog)
			if d.yes != tt.want {
				t.Errorf("trusted = %v, want %v", d.yes, tt.want)
			}
			if d.always != tt.wantAlways {
				t.Errorf("always = %v, want %v", d.always, tt.wantAlways)
			}
		})
	}
}

func TestTrustDialogView(t *testing.T) {
	const box = "always trust this project, even when its .lanes.toml changes"
	tests := []struct {
		name string
		tabs int
		want []string
	}{
		{"unchecked", 0, []string{
			"/some/project", ".lanes.toml", "shell commands", "Type yes to trust it", "NO",
			"[ ] " + box, "enter confirm · tab toggle always · esc quit",
		}},
		{"checked", 1, []string{"[x] " + box}},
		{"unchecked again", 2, []string{"[ ] " + box}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m tea.Model = newTrustDialog("/some/project")
			for range tt.tabs {
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
			}
			v := ansi.Strip(m.View())
			for _, want := range tt.want {
				if !strings.Contains(v, want) {
					t.Errorf("view lacks %q:\n%s", want, v)
				}
			}
			if strings.Index(v, box) < strings.Index(v, "Type yes to trust it") {
				t.Errorf("the checkbox is not under the yes input:\n%s", v)
			}
		})
	}
}

func TestReloadChecksTrust(t *testing.T) {
	const before, after = "# a\n", "# b\n"
	tests := []struct {
		name        string
		always      bool    // the root is trusted always, not by hash
		trustAfter  bool    // the user trusted the changed .lanes.toml (e.g. by restarting lanes)
		msg         tea.Msg // nil = a reload from the tick or ctrl-r
		wantApplied bool
		wantTrusted bool // trust.json trusts the changed file afterwards
	}{
		{"untrusted change is kept out", false, false, nil, false, false},
		{"trusted change is applied", false, true, nil, true, true},
		{":edit project trusts and applies it", false, false, editorDone{project: true}, true, true},
		{":edit of the global file does not trust it", false, false, editorDone{}, false, false},
		{"always-trusted change is applied", true, false, nil, true, true},
		{":edit project keeps always", true, false, editorDone{project: true}, true, true},
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
			if tt.always {
				if err := trust.TrustAlways(dir); err != nil {
					t.Fatal(err)
				}
			}
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
			// a hash no file has: only an always entry trusts it
			if ok, err := trust.IsTrusted(dir, "other"); err != nil || ok != tt.always {
				t.Errorf("always = %v (err %v), want %v", ok, err, tt.always)
			}
		})
	}
}
