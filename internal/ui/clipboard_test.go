package ui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestCopyIDs(t *testing.T) {
	tests := []struct {
		name       string
		marked     []string
		cursor     int
		wantCopied string // "" = nothing copied
		wantNotice string
	}{
		{name: "cursor item", cursor: 1, wantCopied: "2", wantNotice: "copied 2"},
		{name: "marks in list order, cursor ignored", marked: []string{"3", "1"}, cursor: 1, wantCopied: "1 3", wantNotice: "copied 2 IDs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := busyModel(t)
			var got []string
			m.clip = func(s string) error { got = append(got, s); return nil }
			m.cursor = tt.cursor
			for _, id := range tt.marked {
				m.marks[id] = true
			}
			press(m, "y")
			if len(got) != 1 || got[0] != tt.wantCopied {
				t.Errorf("copied %q, want %q", got, tt.wantCopied)
			}
			if m.notice != tt.wantNotice {
				t.Errorf("notice %q, want %q", m.notice, tt.wantNotice)
			}
			if len(m.marks) != len(tt.marked) {
				t.Errorf("marks changed: %v", m.marks)
			}
		})
	}

	t.Run("empty view does nothing", func(t *testing.T) {
		m, _ := busyModel(t)
		m.views[m.cur].items = nil
		m.clip = func(string) error { t.Error("copied from an empty view"); return nil }
		press(m, "y")
	})
	t.Run("failure is a notice", func(t *testing.T) {
		m, _ := busyModel(t)
		m.clip = func(string) error { return errors.New("boom") }
		press(m, "y")
		if m.notice != "copy: boom" {
			t.Errorf("notice %q", m.notice)
		}
	})
}

func TestCopyText(t *testing.T) {
	tests := []struct {
		name     string
		goos     string
		have     []string // tools "installed"
		wantTool string   // "" = OSC 52
		wantArgs string
	}{
		{"macOS pbcopy", "darwin", []string{"pbcopy", "xclip"}, "pbcopy", ""},
		{"linux prefers wl-copy", "linux", []string{"xsel", "xclip", "wl-copy"}, "wl-copy", ""},
		{"linux xclip over xsel", "linux", []string{"xsel", "xclip"}, "xclip", "-selection clipboard"},
		{"linux xsel", "linux", []string{"xsel"}, "xsel", "-b"},
		{"none: OSC 52", "linux", nil, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			look := func(n string) (string, error) {
				for _, h := range tt.have {
					if h == n {
						return "/bin/" + n, nil
					}
				}
				return "", errors.New("not found")
			}
			var tool, args, text string
			run := func(p string, a []string, s string) error {
				tool, args, text = strings.TrimPrefix(p, "/bin/"), strings.Join(a, " "), s
				return nil
			}
			var osc bytes.Buffer
			if err := copyText(tt.goos, look, run, &osc, "!1 !2"); err != nil {
				t.Fatal(err)
			}
			if tool != tt.wantTool || args != tt.wantArgs {
				t.Errorf("ran %q %q, want %q %q", tool, args, tt.wantTool, tt.wantArgs)
			}
			if tt.wantTool != "" && (text != "!1 !2" || osc.Len() != 0) {
				t.Errorf("text %q, osc %q", text, osc.String())
			}
			// base64("!1 !2") = IQ== prefix check
			if tt.wantTool == "" && osc.String() != "\x1b]52;c;ITEgITI=\a" {
				t.Errorf("osc %q", osc.String())
			}
		})
	}
}
