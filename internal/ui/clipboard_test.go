package ui

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/item"
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

func TestCopyURLs(t *testing.T) {
	urls := map[string]string{"1": "https://x/1", "2": "ftp://x/2", "3": ""}
	tests := []struct {
		name       string
		marked     []string
		cursor     int
		clipErr    error
		wantCopied string // "" = clipboard not called
		wantNotice string
	}{
		{name: "cursor item", cursor: 0, wantCopied: "https://x/1", wantNotice: "copied https://x/1"},
		{name: "non-http URL as is", cursor: 1, wantCopied: "ftp://x/2", wantNotice: "copied ftp://x/2"},
		{name: "marks in list order, newline-separated", marked: []string{"2", "1"}, cursor: 2, wantCopied: "https://x/1\nftp://x/2", wantNotice: "copied 2 URLs"},
		{name: "marked items without URL are skipped", marked: []string{"3", "1"}, wantCopied: "https://x/1", wantNotice: "copied https://x/1"},
		{name: "cursor item without URL", cursor: 2, wantNotice: "nothing to copy: no URL"},
		{name: "marked items none with URL", marked: []string{"3"}, wantNotice: "nothing to copy: no URL"},
		{name: "clipboard error", cursor: 0, clipErr: errors.New("boom"), wantCopied: "https://x/1", wantNotice: "copy: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := busyModel(t)
			its := mkItems("1", "2", "3")
			for i := range its {
				its[i].URL = urls[its[i].ID]
			}
			m.views[m.cur].items = its
			var got []string
			m.clip = func(s string) error { got = append(got, s); return tt.clipErr }
			m.cursor = tt.cursor
			for _, id := range tt.marked {
				m.marks[id] = true
			}
			press(m, "Y")
			if tt.wantCopied == "" && len(got) != 0 || tt.wantCopied != "" && (len(got) != 1 || got[0] != tt.wantCopied) {
				t.Errorf("copied %q, want %q", got, tt.wantCopied)
			}
			if !strings.Contains(m.notice, tt.wantNotice) {
				t.Errorf("notice %q, want %q", m.notice, tt.wantNotice)
			}
			if len(m.marks) != len(tt.marked) {
				t.Errorf("marks changed: %v", m.marks)
			}
		})
	}

	t.Run("builtin wins over a keymap binding", func(t *testing.T) {
		m, _ := busyModel(t)
		m.views[m.cur].items[0].URL = "https://x/1"
		m.cfg.Keymap = map[string]string{"Y": "review"}
		var got []string
		m.clip = func(s string) error { got = append(got, s); return nil }
		cmd := press(m, "Y")
		if cmd != nil || len(m.busy) != 0 || len(got) != 1 {
			t.Errorf("cmd %v, busy %v, copied %q; want the copy only", cmd, m.busy, got)
		}
	})
	t.Run("queue tab", func(t *testing.T) {
		m, _ := busyModel(t)
		m.views[m.cur].items[0].URL = "https://x/1"
		var got []string
		m.clip = func(s string) error { got = append(got, s); return nil }
		press(m, "Q")
		m.views[m.cur].items = []item.Item{{ID: "1", URL: "https://x/1"}}
		press(m, "Y")
		if len(got) != 1 || got[0] != "https://x/1" {
			t.Errorf("copied %q on the queue tab", got)
		}
	})
}

// TestDetailCopyURL checks Y on the detail screen: the URL of the thread under the cursor, else the item's.
func TestDetailCopyURL(t *testing.T) {
	th := func(file, url string) item.Thread {
		return item.Thread{File: file, Line: 1, URL: url, Comments: []item.Comment{{Author: "ann", Body: file + "-body"}}}
	}
	both := []item.Thread{th("a.go", "https://x/7#note_1"), th("b.go", "https://x/7#note_2")}
	tests := []struct {
		name string
		prov *detailProv
		keys []string
		want string
	}{
		{"first thread", &detailProv{d: item.Detail{Threads: both}}, []string{"Y"}, "https://x/7#note_1"},
		{"thread under the cursor", &detailProv{d: item.Detail{Threads: both}}, []string{"J", "Y"}, "https://x/7#note_2"},
		{"thread without URL falls back to the item", &detailProv{d: item.Detail{Threads: []item.Thread{th("a.go", "")}}}, []string{"Y"}, "https://x/7"},
		{"no threads copies the item URL", &detailProv{d: item.Detail{Description: "body"}}, []string{"Y"}, "https://x/7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := detailModel(t, tt.prov)
			var got []string
			m.clip = func(s string) error { got = append(got, s); return nil }
			_, cmd := m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
			if cmd != nil {
				m.Update(cmd())
			}
			nav(m, tt.keys...)
			if m.mode != modeDetail {
				t.Fatalf("mode = %d, want detail", m.mode)
			}
			if len(got) != 1 || got[0] != tt.want {
				t.Errorf("copied %q, want %q", got, tt.want)
			}
		})
	}
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
