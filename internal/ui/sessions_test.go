package ui

import (
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/session"
)

func TestSessionRows(t *testing.T) {
	labels := map[string]string{"1": "1 first", "2": "2 second"}
	labelOf := func(key string) string {
		if l, ok := labels[key]; ok {
			return l
		}
		return key
	}
	s := func(name, key string) session.Info { return session.Info{Name: name, ItemKey: key} }
	tests := []struct {
		name   string
		infos  []session.Info
		inView map[string]bool
		want   []sessionRow
	}{
		{"none", nil, nil, nil},
		{"known item in a view", []session.Info{s("fix~1", "1")}, map[string]bool{"1": true},
			[]sessionRow{{s("fix~1", "1"), "1 first", false}}},
		{"known item in no view", []session.Info{s("fix~2", "2")}, map[string]bool{"1": true},
			[]sessionRow{{s("fix~2", "2"), "2 second", true}}},
		{"unknown item falls back to the key", []session.Info{s("fix~9", "9")}, nil,
			[]sessionRow{{s("fix~9", "9"), "9", true}}},
		{"order kept", []session.Info{s("b~2", "2"), s("a~1", "1"), s("c~9", "9")}, map[string]bool{"1": true},
			[]sessionRow{{s("b~2", "2"), "2 second", true}, {s("a~1", "1"), "1 first", false}, {s("c~9", "9"), "9", true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sessionRows(tt.infos, labelOf, tt.inView); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func sessModel(t *testing.T, infos ...session.Info) *Model {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	cfg := &config.Config{}
	cfg.Settings.NoNotify = true
	m := &Model{
		cfg:        cfg,
		sess:       &session.Manager{}, // never run: the tests only inspect the returned commands
		sessions:   infos,
		sessByItem: map[string][]session.Info{},
		marks:      map[string]bool{},
		views:      []*view{{cfg: config.View{Name: "v"}, items: []item.Item{{ID: "1", Title: "first"}}}},
		data:       map[string][]item.Item{},
		width:      120,
		height:     20,
	}
	return m
}

func names(n ...string) []session.Info {
	out := make([]session.Info, len(n))
	for i, s := range n {
		out[i] = session.Info{Name: s, ItemKey: s}
	}
	return out
}

func TestSessionsCursorSticks(t *testing.T) {
	tests := []struct {
		name   string
		before []string
		cursor int
		after  []string
		want   int
	}{
		{"same session moves down", []string{"a", "b"}, 1, []string{"c", "a", "b"}, 2},
		{"same session moves up", []string{"a", "b", "c"}, 2, []string{"c"}, 0},
		{"selected gone, clamps", []string{"a", "b", "c"}, 2, []string{"a", "b"}, 1},
		{"all gone", []string{"a"}, 0, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := sessModel(t, names(tt.before...)...)
			m.mode, m.sessCursor = modeSessions, tt.cursor
			m.onSessions(sessionsMsg{infos: names(tt.after...)})
			if m.sessCursor != tt.want {
				t.Errorf("cursor = %d, want %d", m.sessCursor, tt.want)
			}
		})
	}
}

func press(m *Model, keys ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		switch k {
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		}
		_, cmd = m.key(msg)
	}
	return cmd
}

func TestSessionsScreenKeys(t *testing.T) {
	tests := []struct {
		name   string
		keys   []string
		mode   mode
		cursor int
	}{
		{"S opens", []string{"S"}, modeSessions, 0},
		{"q closes", []string{"S", "q"}, modeNormal, 0},
		{"esc closes", []string{"S", "esc"}, modeNormal, 0},
		{"S closes", []string{"S", "S"}, modeNormal, 0},
		{"j moves", []string{"S", "j"}, modeSessions, 1},
		{"j stops at the end", []string{"S", "j", "j", "j"}, modeSessions, 2},
		{"G bottom, k up", []string{"S", "G", "k"}, modeSessions, 1},
		{"gg top", []string{"S", "G", "g", "g"}, modeSessions, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := sessModel(t, names("a", "b", "c")...)
			press(m, tt.keys...)
			if m.mode != tt.mode || m.sessCursor != tt.cursor {
				t.Errorf("mode %d cursor %d, want mode %d cursor %d", m.mode, m.sessCursor, tt.mode, tt.cursor)
			}
		})
	}
}

func TestSessionsCommand(t *testing.T) {
	m := sessModel(t)
	m.runCommand("sessions")
	if m.mode != modeSessions {
		t.Fatalf("mode = %d, want sessions screen", m.mode)
	}
	if v := m.View(); !strings.Contains(v, "no sessions") {
		t.Errorf("empty screen lacks %q:\n%s", "no sessions", v)
	}
}

func TestSessionsScreenView(t *testing.T) {
	m := sessModel(t, session.Info{Name: "fix~1", Action: "fix", ItemKey: "1"},
		session.Info{Name: "fix~9", Action: "fix", ItemKey: "9", Dead: true})
	m.data["p"] = []item.Item{{ID: "1", Title: "first"}}
	m.mode = modeSessions
	lines := strings.Split(m.View(), "\n")
	var one, nine string
	for _, l := range lines {
		switch {
		case strings.Contains(l, "1 first"):
			one = l
		case strings.Contains(l, "✓ fix"):
			nine = l
		}
	}
	if one == "" || strings.Contains(one, "not in any view") || !strings.Contains(one, "▶ fix") {
		t.Errorf("row of item in a view = %q", one)
	}
	if !strings.Contains(nine, " 9 ") || !strings.Contains(nine, "(not in any view)") {
		t.Errorf("row of vanished item = %q", nine)
	}
}

func TestSessionsClose(t *testing.T) {
	tests := []struct {
		name    string
		dead    bool
		answer  string
		cmd     bool // a kill command is returned
		confirm bool // the screen asks first
	}{
		{"dead closes directly", true, "", true, false},
		{"running asks, yes kills", false, "y", true, true},
		{"running asks, no keeps", false, "n", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := sessModel(t, session.Info{Name: "fix~1", Action: "fix", ItemKey: "1", Dead: tt.dead})
			cmd := press(m, "S", "x")
			if asked := m.mode == modeConfirm; asked != tt.confirm {
				t.Fatalf("asked = %v, want %v", asked, tt.confirm)
			}
			if tt.confirm {
				if !strings.Contains(m.confirm.prompt, "Kill 1 running session") {
					t.Errorf("prompt = %q", m.confirm.prompt)
				}
				cmd = press(m, tt.answer)
			}
			if (cmd != nil) != tt.cmd {
				t.Errorf("kill command returned = %v, want %v", cmd != nil, tt.cmd)
			}
			if m.mode != modeSessions {
				t.Errorf("mode = %d, want back on the sessions screen", m.mode)
			}
		})
	}
}

func TestNextAttention(t *testing.T) {
	at := func(name, claude string, created int64, dead bool) session.Info {
		return session.Info{Name: name, Claude: claude, Created: time.Unix(created, 0), Dead: dead}
	}
	// newest first, as session.List returns them
	mixed := []session.Info{
		at("w-new", "waiting", 40, false),
		at("a-new", "asking", 30, false),
		at("busy", "working", 25, false),
		at("dead", "asking", 15, true),
		at("w-old", "waiting", 20, false),
		at("a-old", "asking", 10, false),
	}
	tests := []struct {
		name  string
		infos []session.Info
		last  string
		want  string
		ok    bool
	}{
		{"empty", nil, "", "", false},
		{"none qualifying", []session.Info{at("busy", "working", 1, false), at("dead", "waiting", 2, true)}, "", "", false},
		{"oldest asking first", mixed, "", "a-old", true},
		{"after the last jumped", mixed, "a-old", "a-new", true},
		{"asking before waiting", mixed, "a-new", "w-old", true},
		{"waiting oldest first", mixed, "w-old", "w-new", true},
		{"wraps around", mixed, "w-new", "a-old", true},
		{"last no longer qualifying", mixed, "busy", "a-old", true},
		{"only the last qualifies", []session.Info{at("a", "asking", 1, false)}, "a", "a", true},
	}
	for _, tt := range tests {
		got, ok := nextAttention(tt.infos, tt.last)
		if got != tt.want || ok != tt.ok {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", tt.name, got, ok, tt.want, tt.ok)
		}
	}
}

func TestPruneNotice(t *testing.T) {
	tests := []struct {
		name string
		n    int
		err  error
		want string
	}{
		{"none", 0, nil, "pruned 0 sessions"},
		{"one", 1, nil, "pruned 1 session"},
		{"several", 2, nil, "pruned 2 sessions"},
		{"error appended", 1, errors.New("exit status 1"), "pruned 1 session; exit status 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pruneNotice(tt.n, tt.err); got != tt.want {
				t.Errorf("pruneNotice(%d, %v) = %q, want %q", tt.n, tt.err, got, tt.want)
			}
		})
	}
}

func TestPruneCommand(t *testing.T) {
	tests := []struct {
		line    string
		wantCmd bool
		notice  string
	}{
		{"prune", true, ""},
		{"prune failed", true, ""},
		{"prune bogus", false, "usage: :prune [failed]"},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			m := sessModel(t)
			_, cmd := m.runCommand(tt.line)
			if (cmd != nil) != tt.wantCmd || m.notice != tt.notice {
				t.Errorf("cmd %v, notice %q; want cmd %v, notice %q", cmd != nil, m.notice, tt.wantCmd, tt.notice)
			}
		})
	}
}

func TestSessionsViewHighlightsCursorRow(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(0) // termenv.TrueColor: render the colors the test compares
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	bgRe := regexp.MustCompile(`48;[0-9;]+`)
	bg := bgRe.FindString(lipgloss.NewStyle().Background(cSelBg).Render("x"))
	if bg == "" {
		t.Fatal("test needs a color profile: styles rendered plain")
	}
	infos := []session.Info{
		{Name: "a~1", ItemKey: "1", Created: time.Now().Add(-time.Hour)},
		{Name: "b~9", ItemKey: "9", Created: time.Now().Add(-time.Hour)}, // in no view: orphan hint
	}
	rowLine := func(m *Model, i int) string { return strings.Split(m.sessionsView(), "\n")[2+i] }
	for cursor := range infos {
		m := sessModel(t, infos...)
		m.sessCursor = cursor
		for i := range infos {
			line := rowLine(m, i)
			if i != cursor {
				if strings.Contains(line, bg) || strings.Contains(line, "▸") {
					t.Errorf("cursor %d: row %d = %q, want no highlight", cursor, i, line)
				}
				continue
			}
			if !strings.Contains(line, "▸ ") {
				t.Errorf("cursor %d: row %d = %q, lost the marker", cursor, i, line)
			}
			// every visible chunk (text between resets) must carry the background
			for _, chunk := range strings.Split(line, "\x1b[0m") {
				if ansi.Strip(chunk) != "" && !strings.Contains(chunk, bg) {
					t.Errorf("cursor %d: chunk %q of %q has no background", cursor, chunk, line)
				}
			}
			other := rowLine(m, 1-i)
			if got, want := ansi.StringWidth(line), ansi.StringWidth(other); got != want {
				t.Errorf("cursor %d: row width %d, other row %d", cursor, got, want)
			}
		}
	}
}

func TestSessionsRowFillsScreenWidth(t *testing.T) {
	long := strings.Repeat("long label ", 20)
	for _, width := range []int{60, 120} {
		infos := []session.Info{
			{Name: "a~1", ItemKey: "1", Created: time.Now().Add(-time.Hour)},
			{Name: "b~2", ItemKey: long, Created: time.Now().Add(-time.Hour)},
		}
		for cursor := range infos {
			m := sessModel(t, infos...)
			m.width = width
			m.sessCursor = cursor
			lines := strings.Split(m.sessionsView(), "\n")
			for i := range infos {
				if got := ansi.StringWidth(lines[2+i]); got != width {
					t.Errorf("width %d cursor %d: row %d is %d columns wide: %q", width, cursor, i, got, lines[2+i])
				}
			}
			if !strings.Contains(lines[3], "…") {
				t.Errorf("width %d: long label not truncated: %q", width, lines[3])
			}
		}
	}
}
