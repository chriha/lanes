package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The key messages bubbletea v1.3.4 makes of an OSC 11 reply "\x1b]11;rgb:…" ended by ESC \ or BEL.
var (
	keyOSC = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}, Alt: true}  // ESC ]
	keyST  = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\\'}, Alt: true} // ESC \
	keyBEL = tea.KeyMsg{Type: tea.KeyCtrlG}                                 // BEL
)

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// reply is the key messages of the terminal's answer with the given body, ended by end.
func reply(body string, end tea.KeyMsg) []tea.KeyMsg {
	return []tea.KeyMsg{keyOSC, runes(body), end}
}

const (
	lightBody = "11;rgb:ffff/ffff/ffff"
	darkBody  = "11;rgb:1e1e/1e1e/1e1e"
)

// newBgModel is a model in theme mode mode on a terminal that started dark (or light) and answers background
// queries into the returned buffer instead of a real terminal; its provider is fresh, so a focus fetches nothing.
func newBgModel(t *testing.T, mode string, dark bool) (*Model, *bytes.Buffer) {
	t.Helper()
	trueColorTheme(t)
	m := newBackoffModel(t)
	m.fetchedAt["p"] = time.Now()
	out := &bytes.Buffer{}
	m.bgOut = out
	m.getenv = func(k string) string { return map[string]string{"TERM": "xterm-256color"}[k] }
	detectedDark = nil
	lipgloss.SetHasDarkBackground(dark)
	m.cfg.Theme.Mode = mode
	applyTheme(m.cfg.Theme)
	return m, out
}

// focus sends a focus regain and runs its commands; it returns the query's sent message, if one was written.
func focus(t *testing.T, m *Model) (bgSentMsg, bool) {
	t.Helper()
	_, cmd := m.Update(tea.FocusMsg{})
	var sent []bgSentMsg
	var run func(tea.Cmd)
	run = func(c tea.Cmd) {
		if c == nil {
			return
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			for _, c := range msg {
				run(c)
			}
		case bgSentMsg:
			sent = append(sent, msg)
		}
	}
	run(cmd)
	if len(sent) > 1 {
		t.Fatalf("focus wrote %d background queries, want at most one", len(sent))
	}
	if len(sent) == 0 {
		return bgSentMsg{}, false
	}
	m.Update(sent[0])
	return sent[0], true
}

func TestFocusRedetectsBackground(t *testing.T) {
	tests := []struct {
		name      string
		mode      string
		startDark bool
		keys      []tea.KeyMsg // the terminal's answer; nil: none, the capture times out
		wantQuery bool
		wantDark  bool
	}{
		{"system: dark to light, ST", "system", true, reply(lightBody, keyST), true, false},
		{"system: light to dark, BEL", "system", false, reply(darkBody, keyBEL), true, true},
		{"default mode: dark to light", "", true, reply(lightBody, keyBEL), true, false},
		{"system: unchanged", "system", true, reply(darkBody, keyST), true, true},
		{"system: no answer keeps the palette", "system", false, nil, true, false},
		{"system: garbled answer keeps the palette", "system", true, reply("11;rgb:ff/ff", keyST), true, true},
		{"light: not asked", "light", true, nil, false, false},
		{"dark: not asked", "dark", false, nil, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, out := newBgModel(t, tt.mode, tt.startDark)
			sent, ok := focus(t, m)
			if ok != tt.wantQuery {
				t.Fatalf("query written = %v, want %v", ok, tt.wantQuery)
			}
			if got, want := out.String(), map[bool]string{true: "\x1b]11;?\x1b\\"}[tt.wantQuery]; got != want {
				t.Errorf("terminal got %q, want %q", got, want)
			}
			for _, k := range tt.keys {
				m.Update(k)
			}
			if tt.keys == nil {
				m.Update(bgTimeoutMsg{gen: sent.gen})
			}
			if got := lipgloss.HasDarkBackground(); got != tt.wantDark {
				t.Errorf("dark background = %v, want %v", got, tt.wantDark)
			}
			// the selection background is resolved when the styles are built: it shows they were rebuilt
			wantSel := map[bool]string{true: "64;72;113", false: "207;195;240"}[tt.wantDark]
			if got := sTabOn.Render("x"); !strings.Contains(got, wantSel) {
				t.Errorf("active tab = %q, want the selection background %s", got, wantSel)
			}
		})
	}
}

func TestFocusQuerySkippedOutsideATerminalItMayAsk(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"dumb terminal", map[string]string{"TERM": "dumb"}, ""},
		{"screen without tmux", map[string]string{"TERM": "screen-256color"}, ""},
		{"tmux: through passthrough", map[string]string{"TERM": "tmux-256color", "TMUX": "/tmp/tmux-1/default,1,0"},
			"\x1bPtmux;\x1b\x1b]11;?\x1b\x1b\\\x1b\\"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, out := newBgModel(t, "system", true)
			m.getenv = func(k string) string { return tt.env[k] }
			focus(t, m)
			if got := out.String(); got != tt.want {
				t.Errorf("terminal got %q, want %q", got, tt.want)
			}
		})
	}
}

// The reply's keys are tested in search mode, where every key that reaches the app lands in the query.
func TestBackgroundReplyKeys(t *testing.T) {
	tests := []struct {
		name      string
		focus     bool         // a focus regain sent the query first
		keys      []tea.KeyMsg // what reaches the program after it
		timeout   bool         // the capture's timeout fires after the keys
		late      []tea.KeyMsg // what reaches the program after the timeout
		wantQuery string
		wantDark  bool
	}{
		{"reply is swallowed", true, reply(lightBody, keyST), false, nil, "", false},
		{"key typed before the reply passes", true, append([]tea.KeyMsg{runes("a")}, reply(lightBody, keyBEL)...), false, nil,
			"a", false},
		{"key typed after the reply passes", true, append(reply(lightBody, keyBEL), runes("b")), false, nil, "b", false},
		{"breaking key after ESC ] replays in order", true, []tea.KeyMsg{keyOSC, runes("x")}, false, nil, "]x", true},
		{"breaking key in the body replays in order", true, []tea.KeyMsg{keyOSC, runes("11;rgb:ff"), runes("x")}, false, nil,
			"]11;rgb:ffx", true},
		{"terminator before the color breaks", true, []tea.KeyMsg{keyOSC, runes("11;"), keyST}, false, nil, "]11;\\", true},
		{"body split over two messages", true, []tea.KeyMsg{keyOSC, runes("11;rgb:ffff/"), runes("ffff/ffff"), keyBEL}, false,
			nil, "", false},
		{"timeout replays the swallowed keys", true, []tea.KeyMsg{keyOSC, runes("11;rgb:")}, true, nil, "]11;rgb:", true},
		{"reply after the timeout is not swallowed", true, nil, true, reply(lightBody, keyST), "]" + lightBody + "\\", true},
		{"reply without a query is not swallowed", false, reply(lightBody, keyST), false, nil, "]" + lightBody + "\\", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newBgModel(t, "system", true)
			m.Update(runes("/"))
			var sent bgSentMsg
			if tt.focus {
				var ok bool
				if sent, ok = focus(t, m); !ok {
					t.Fatal("focus wrote no background query")
				}
			}
			for _, k := range tt.keys {
				m.Update(k)
			}
			if tt.timeout {
				m.Update(bgTimeoutMsg{gen: sent.gen})
			}
			for _, k := range tt.late {
				m.Update(k)
			}
			if m.query != tt.wantQuery {
				t.Errorf("search query = %q, want %q", m.query, tt.wantQuery)
			}
			if got := lipgloss.HasDarkBackground(); got != tt.wantDark {
				t.Errorf("dark background = %v, want %v", got, tt.wantDark)
			}
		})
	}
}

func TestStaleBackgroundMessagesAreDropped(t *testing.T) {
	m, out := newBgModel(t, "system", true)
	first, _ := focus(t, m)
	m.Update(bgTimeoutMsg{gen: first.gen})
	second, ok := focus(t, m)
	if !ok || second.gen == first.gen {
		t.Fatalf("second focus: query written = %v, gen %d after %d; want a new capture", ok, second.gen, first.gen)
	}
	m.Update(keyOSC)
	m.Update(bgTimeoutMsg{gen: first.gen}) // the first capture's late timeout must not end the second one
	m.Update(runes(lightBody))
	m.Update(keyST)
	if lipgloss.HasDarkBackground() {
		t.Error("the second capture's reply was not applied after the first capture's late timeout")
	}
	if n := strings.Count(out.String(), "\x1b]11;?"); n != 2 {
		t.Errorf("queries written = %d, want 2", n)
	}
}

func TestFocusWhileCapturingWritesNoSecondQuery(t *testing.T) {
	m, out := newBgModel(t, "system", true)
	focus(t, m)
	focus(t, m)
	if n := strings.Count(out.String(), "\x1b]11;?"); n != 1 {
		t.Errorf("queries written = %d, want 1 while the first is unanswered", n)
	}
}
