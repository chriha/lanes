package ui

import (
	"errors"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/filter"
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
		views: []*view{{cfg: config.View{Name: "v"}, items: []item.Item{{ID: "1", Title: "first"}}},
			newQueueView(), newSessionsView()},
		data:   map[string][]item.Item{},
		width:  120,
		height: 20,
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
			press(m, "S")
			m.sessCursor = tt.cursor
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

func TestSessionsTabKeys(t *testing.T) {
	tests := []struct {
		name   string
		keys   []string
		view   string // the current view after keys
		mode   mode
		cursor int
	}{
		{"S switches to it", []string{"S"}, "sessions", modeNormal, 0},
		{"S again stays", []string{"S", "S"}, "sessions", modeNormal, 0},
		{"esc stays", []string{"S", "esc"}, "sessions", modeNormal, 0},
		{"q asks to quit", []string{"S", "q"}, "sessions", modeConfirm, 0},
		{"h goes to the queue tab", []string{"S", "h"}, "queue", modeNormal, 0},
		{"1 goes to the first view", []string{"S", "1"}, "v", modeNormal, 0},
		{"2 is no view: stays", []string{"S", "2"}, "sessions", modeNormal, 0},
		{"j moves", []string{"S", "j"}, "sessions", modeNormal, 1},
		{"j stops at the end", []string{"S", "j", "j", "j"}, "sessions", modeNormal, 2},
		{"G bottom, k up", []string{"S", "G", "k"}, "sessions", modeNormal, 1},
		{"gg top", []string{"S", "G", "g", "g"}, "sessions", modeNormal, 0},
		{"switching back starts at the top", []string{"S", "j", "h", "S"}, "sessions", modeNormal, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := sessModel(t, names("a", "b", "c")...)
			press(m, tt.keys...)
			if v := m.views[m.cur].cfg.Name; v != tt.view || m.mode != tt.mode || m.sessCursor != tt.cursor {
				t.Errorf("view %q mode %d cursor %d, want view %q mode %d cursor %d", v, m.mode, m.sessCursor, tt.view, tt.mode, tt.cursor)
			}
		})
	}
}

func TestSessionsCommand(t *testing.T) {
	m := sessModel(t)
	m.runCommand("sessions")
	if !m.onSessionsTab() || m.mode != modeNormal {
		t.Fatalf("mode = %d, on the sessions view %v; want the sessions view", m.mode, m.onSessionsTab())
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "no sessions") {
		t.Errorf("empty view lacks %q:\n%s", "no sessions", v)
	}
	if !strings.Contains(v, "sessions 0") {
		t.Errorf("tab bar lacks %q:\n%s", "sessions 0", v)
	}
}

// The sessions view is the last tab, after the queue, set apart by a divider; its label counts every session and,
// in the attention colour, the live ones asking or waiting.
func TestSessionsTabLabel(t *testing.T) {
	at := func(name, claude string, dead bool) session.Info {
		return session.Info{Name: name, ItemKey: name, Claude: claude, Dead: dead}
	}
	tests := []struct {
		name  string
		infos []session.Info
		want  string
	}{
		{"none", nil, "v 0 ¦ queue ¦ sessions 0"},
		{"no attention", []session.Info{at("a", "working", false), at("b", "", true)}, "v 0 ¦ queue ¦ sessions 2"},
		{"asking and waiting count, dead ones not",
			[]session.Info{at("a", "asking", false), at("b", "waiting", false), at("c", "asking", true), at("d", "working", false)},
			"v 0 ¦ queue ¦ sessions 4 2"},
	}
	for _, tt := range tests {
		for _, active := range []bool{false, true} {
			m := sessModel(t, tt.infos...)
			m.views[0].items = nil
			if active {
				press(m, "S")
			}
			got := strings.Join(strings.Fields(ansi.Strip(m.header())), " ")
			if !strings.Contains(got, tt.want) || !strings.HasSuffix(strings.SplitN(got, "sessions", 2)[0], "¦ queue ¦ ") {
				t.Errorf("%s (active %v): header %q lacks %q", tt.name, active, got, tt.want)
			}
		}
	}
}

// The label follows the 1 s session refresh.
func TestSessionsTabLabelUpdates(t *testing.T) {
	m := sessModel(t, names("a")...)
	if h := ansi.Strip(m.header()); !strings.Contains(h, "sessions 1") {
		t.Fatalf("header %q lacks sessions 1", h)
	}
	m.onSessions(sessionsMsg{infos: names("a", "b", "c")})
	if h := ansi.Strip(m.header()); !strings.Contains(h, "sessions 3") {
		t.Errorf("header %q lacks sessions 3 after the refresh", h)
	}
}

func TestSessionsTabView(t *testing.T) {
	m := sessModel(t, session.Info{Name: "fix~1", Action: "fix", ItemKey: "1", Created: time.Now()},
		session.Info{Name: "fix~9", Action: "fix", ItemKey: "9", Dead: true, Created: time.Now().Add(-time.Hour)})
	m.data["p"] = []item.Item{{ID: "1", Title: "first"}}
	f, err := filter.Compile("true")
	if err != nil {
		t.Fatal(err)
	}
	m.views[0].cfg.Provider, m.views[0].filter = "p", f
	press(m, "S")
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	one, nine := -1, -1
	for i, l := range lines {
		switch {
		case strings.Contains(l, "1 first"):
			one = i
		case strings.Contains(l, "✓ fix"):
			nine = i
		}
	}
	if one < 0 || strings.Contains(lines[one], "not in any view") || !strings.Contains(lines[one], "▶ fix") {
		t.Fatalf("row of item in a view missing or wrong:\n%s", strings.Join(lines, "\n"))
	}
	if nine < 0 || !strings.Contains(lines[nine], " 9 ") || !strings.Contains(lines[nine], "(not in any view)") {
		t.Fatalf("row of vanished item missing or wrong:\n%s", strings.Join(lines, "\n"))
	}
	if nine < one {
		t.Errorf("rows out of order: newest first wanted\n%s", strings.Join(lines, "\n"))
	}
	if !strings.Contains(lines[0], "sessions 2") {
		t.Errorf("tab bar %q lacks sessions 2", lines[0])
	}
}

// enter on the sessions view jumps into the session under the cursor.
func TestSessionsTabJump(t *testing.T) {
	m := sessModel(t, session.Info{Name: "fix~1", ItemKey: "1"}, session.Info{Name: "fix~2", ItemKey: "2"})
	log := paneTmux(t, 0)
	mgr, err := session.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	m.sess = mgr
	runAll(press(m, "S", "j", "enter"))
	raw, _ := os.ReadFile(log)
	if !strings.Contains(string(raw), "fix~2") || strings.Contains(string(raw), "fix~1") {
		t.Errorf("tmux calls:\n%s\nwant a jump into fix~2 only", raw)
	}
}

// Item keys and configured actions do nothing on the sessions view, even with items marked in another view.
func TestSessionsTabIgnoresItemKeys(t *testing.T) {
	for _, key := range []string{"o", "y", "N", "f", "t", "d", "i", "z", "m", "F", "V", " ", "/"} {
		t.Run(key, func(t *testing.T) {
			m := sessModel(t, session.Info{Name: "fix~1", Action: "fix", ItemKey: "1"})
			m.cfg.Action = map[string]config.Action{"fix": {Run: "echo fix"}}
			m.cfg.Keymap = map[string]string{"f": "fix"}
			m.marks["1"] = true
			var opened, copied []string
			m.open = func(u string) error { opened = append(opened, u); return nil }
			m.clip = func(s string) error { copied = append(copied, s); return nil }
			press(m, "S")
			cmd := press(m, key)
			if cmd != nil || opened != nil || copied != nil || m.mode != modeNormal || len(m.busy) != 0 || !m.onSessionsTab() {
				t.Errorf("%q: cmd %v, opened %v, copied %v, mode %d, busy %v, on the sessions view %v; want nothing",
					key, cmd != nil, opened, copied, m.mode, m.busy, m.onSessionsTab())
			}
			if !m.marks["1"] || len(m.marks) != 1 {
				t.Errorf("%q: marks = %v, want them untouched", key, m.marks)
			}
		})
	}
}

func TestSessionsClose(t *testing.T) {
	tests := []struct {
		name    string
		dead    bool
		answer  string
		cmd     bool // a kill command is returned
		confirm bool // the view asks first
	}{
		{"dead asks, yes closes", true, "y", true, true},
		{"dead asks, no keeps", true, "n", false, true},
		{"running asks, yes kills", false, "y", true, true},
		{"running asks, no keeps", false, "esc", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := sessModel(t, session.Info{Name: "fix~1", Action: "fix", ItemKey: "1", Dead: tt.dead})
			cmd := press(m, "S", "x")
			if asked := m.mode == modeConfirm; asked != tt.confirm {
				t.Fatalf("asked = %v, want %v", asked, tt.confirm)
			}
			if tt.confirm {
				want := "Close 1 session(s)?"
				if !tt.dead {
					want = "Close 1 session(s) (1 running)?"
				}
				if m.confirm.prompt != want {
					t.Errorf("prompt = %q, want %q", m.confirm.prompt, want)
				}
				if v := ansi.Strip(m.View()); !strings.Contains(v, want) || !strings.Contains(v, "fix~1") && !strings.Contains(v, " 1 ") {
					t.Errorf("view while asking lacks the prompt or the row:\n%s", v)
				}
				cmd = press(m, tt.answer)
			}
			if (cmd != nil) != tt.cmd {
				t.Errorf("kill command returned = %v, want %v", cmd != nil, tt.cmd)
			}
			if m.mode != modeNormal || !m.onSessionsTab() {
				t.Errorf("mode = %d, on the sessions view %v; want back on the sessions view", m.mode, m.onSessionsTab())
			}
		})
	}
}

func TestListClose(t *testing.T) {
	tests := []struct {
		name   string
		marked []string
		keys   []string // after x
		prompt string   // "" = no prompt
		notice string
		cmd    bool // a kill command is returned
	}{
		{"finished only, yes closes", nil, []string{"y"}, "Close 1 session(s)?", "", true},
		{"finished only, enter closes", nil, []string{"enter"}, "Close 1 session(s)?", "", true},
		{"finished only, no keeps", nil, []string{"n"}, "Close 1 session(s)?", "cancelled", false},
		{"marked mixed counts running", []string{"1", "2"}, []string{"y"}, "Close 3 session(s) (1 running)?", "", true},
		{"no sessions asks nothing", []string{"3"}, nil, "", "no sessions here", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := sessModel(t)
			m.views[0].items = []item.Item{{ID: "1"}, {ID: "2"}, {ID: "3"}}
			m.sessByItem["1"] = []session.Info{{Name: "fix~1", ItemKey: "1", Dead: true}}
			m.sessByItem["2"] = []session.Info{{Name: "fix~2", ItemKey: "2"}, {Name: "rev~2", ItemKey: "2", Dead: true}}
			for _, id := range tt.marked {
				m.marks[id] = true
			}
			cmd := press(m, "x")
			if asked := m.mode == modeConfirm; asked != (tt.prompt != "") {
				t.Fatalf("asked = %v, want %v", asked, tt.prompt != "")
			}
			if tt.prompt != "" && m.confirm.prompt != tt.prompt {
				t.Errorf("prompt = %q, want %q", m.confirm.prompt, tt.prompt)
			}
			if tt.keys != nil {
				cmd = press(m, tt.keys...)
			}
			if (cmd != nil) != tt.cmd {
				t.Errorf("kill command returned = %v, want %v", cmd != nil, tt.cmd)
			}
			if m.notice != tt.notice {
				t.Errorf("notice = %q, want %q", m.notice, tt.notice)
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
	useSelection(t, "#112233")
	bgRe := regexp.MustCompile(`48;[0-9;]+`)
	bg := bgRe.FindString(lipgloss.NewStyle().Background(lipgloss.Color("#112233")).Render("x"))
	if bg == "" {
		t.Fatal("test needs a color profile: styles rendered plain")
	}
	infos := []session.Info{
		{Name: "a~1", ItemKey: "1", Created: time.Now().Add(-time.Hour)},
		{Name: "b~9", ItemKey: "9", Created: time.Now().Add(-time.Hour)}, // in no view: orphan hint
	}
	rowLine := func(m *Model, i int) string { return strings.Split(m.sessionsList(), "\n")[i] }
	for cursor := range infos {
		m := sessModel(t, infos...)
		m.sessCursor = cursor
		for i := range infos {
			line := rowLine(m, i)
			if i != cursor {
				if strings.Contains(line, bg) {
					t.Errorf("cursor %d: row %d = %q, want no highlight", cursor, i, line)
				}
				continue
			}
			// every visible chunk (text between resets) must carry the background
			for _, chunk := range strings.Split(line, "\x1b[0m") {
				if ansi.Strip(chunk) != "" && !strings.Contains(chunk, bg) {
					t.Errorf("cursor %d: chunk %q of %q has no background", cursor, chunk, line)
				}
			}
			if strings.Contains(line, "▸") {
				t.Errorf("cursor %d: row %d = %q, has a cursor glyph", cursor, i, line)
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
			lines := strings.Split(m.sessionsList(), "\n")
			for i := range infos {
				if got := ansi.StringWidth(lines[i]); got != width {
					t.Errorf("width %d cursor %d: row %d is %d columns wide: %q", width, cursor, i, got, lines[i])
				}
			}
			if !strings.Contains(lines[1], "…") {
				t.Errorf("width %d: long label not truncated: %q", width, lines[1])
			}
		}
	}
}
