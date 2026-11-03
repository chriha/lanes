package flowview

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/session"
)

var update = flag.Bool("update", false, "rewrite the golden frames in testdata")

// fixtureState is a working session with a lead, three agents over three depths, one done and one failed.
func fixtureState() State {
	now := time.Date(2026, 11, 1, 14, 30, 0, 0, time.UTC)
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	return State{
		Title: "issue-196",
		Hook:  "working",
		Start: ago(12*time.Minute + 3*time.Second),
		Now:   now,
		Phase: 1,
		// 30% of the 5h window over 1,774,565 tokens: lead 21%, implementer 7%, triager 0%, code-reviewer 2%,
		// validator 0%.
		Usage5h:      30,
		Usage5hKnown: true,
		Snap: session.Snapshot{
			Lead: session.Lead{
				Model:     "claude-opus-4-1-20250805",
				Tool:      "Agent",
				ToolArg:   "implementer",
				Turns:     12,
				Calls:     48,
				ToolCalls: map[string]int{"Read": 20, "Bash": 14, "Agent": 9, "Edit": 5},
				Tokens:    1_234_567,
				LastText:  "Slice 2 is next: the layered layout and the static panels.",

				Effort:       "high",
				Ctx:          61,
				TokensIn:     12_345,
				TokensOut:    98_765,
				TokensCache:  1_123_457,
				AdvisorModel: "claude-opus-4-1",
				AdvisorCalls: 3,
			},
			ToolTimes: toolTimes(now, map[int]int{20: 4, 14: 2, 10: 1, 7: 6, 5: 2, 3: 4, 2: 8, 1: 5, 0: 3}),
			Files:     []string{"render_panels.go", "state.go", "layout.go", "README.md"},
			Agents: []session.Agent{
				{ID: "a1", Name: "implementer", Model: "claude-opus-4-1", Task: "slice 2 of #196", Text: "Running the whole test suite now", Start: ago(9 * time.Minute), Tool: "Bash", ToolArg: "go test ./...", Ctx: 42, Effort: "high", Tokens: 412_345, Calls: 37},
				{ID: "a2", Name: "triager", Model: "claude-haiku-4-5", Task: "triage #201", Text: "Labelled it as a bug", Status: session.AgentDone, Start: ago(11 * time.Minute), End: ago(10 * time.Minute), Ctx: 7, Effort: "low", Tokens: 23_456, Calls: 8},
				{ID: "a3", Parent: "a1", Name: "code-reviewer", Model: "claude-sonnet-4-5", Task: "review the diff", Start: ago(2 * time.Minute), Tool: "Grep", ToolArg: "drawAgent", Ctx: -1, Effort: "medium", Tokens: 98_765, Calls: 21},
				{ID: "a4", Parent: "a3", Name: "validator", Model: "claude-sonnet-4-5", Task: "validate a finding", Status: session.AgentError, Start: ago(90 * time.Second), End: ago(30 * time.Second), Ctx: 12, Tokens: 5_432, Calls: 3},
			},
		},
		Log: []LogLine{
			{At: ago(5 * time.Minute), Actor: "lead", Model: "claude-opus-4-1", Text: "spawned implementer"},
			{At: ago(2 * time.Minute), Actor: "implementer", Model: "claude-opus-4-1", Text: "spawned code-reviewer"},
			{At: ago(90 * time.Second), Actor: "code-reviewer", Model: "claude-sonnet-4-5", Text: "spawned validator"},
			{At: ago(30 * time.Second), Actor: "validator", Model: "claude-sonnet-4-5", Text: "failed", Bad: true},
			{At: ago(20 * time.Second), Actor: "code-reviewer", Model: "claude-sonnet-4-5", Text: "Bash go test ./internal/...", Bad: true},
			{At: ago(10 * time.Second), Actor: "lead", Model: "claude-opus-4-1", Text: "Slice 2 is next: the layered layout"},
		},
	}
}

// withProcs adds a sampled process tree to st: the pane's shell, claude, the implementer's go test (run by a shell
// that evals it) with its compiler, an MCP server of the lead and gopls on its first sample.
func withProcs(st State) State {
	st.Procs = []session.Proc{
		{PID: 100, PPID: 1, HasCPU: true, RSS: 4 << 20, Command: "-zsh"},
		{PID: 101, PPID: 100, CPU: 35.2, HasCPU: true, RSS: 420 << 20, Command: "/Users/me/.local/bin/claude --model opus"},
		{PID: 200, PPID: 101, CPU: 0.4, HasCPU: true, RSS: 3 << 20, Command: "/bin/zsh -c -l eval 'go test ./...'"},
		{PID: 201, PPID: 200, CPU: 180, HasCPU: true, RSS: 1536 << 20, Command: "go test ./..."},
		{PID: 202, PPID: 201, CPU: 95.6, HasCPU: true, RSS: 210 << 20, Command: "/usr/local/go/pkg/tool/darwin_arm64/compile -o x.a"},
		{PID: 300, PPID: 101, CPU: 2, HasCPU: true, RSS: 80 << 20, Command: "node mcp-server.js"},
		{PID: 301, PPID: 101, RSS: 900 << 10, Command: "gopls"},
	}
	return st
}

// toolTimes returns tool call times before now: perMinute maps minutes ago to the calls in that minute.
func toolTimes(now time.Time, perMinute map[int]int) []time.Time {
	var out []time.Time
	for ago := 30; ago >= 0; ago-- {
		for i := 0; i < perMinute[ago]; i++ {
			out = append(out, now.Add(-time.Duration(ago)*time.Minute-time.Duration(i+1)*time.Second))
		}
	}
	return out
}

func TestViewGolden(t *testing.T) {
	theme := NewTheme(Palette{
		Text: lipgloss.AdaptiveColor{Light: "#000000", Dark: "#ffffff"}, Dim: lipgloss.AdaptiveColor{Light: "#888888", Dark: "#666666"},
		Opus: lipgloss.AdaptiveColor{Light: "#7744cc", Dark: "#aa88ff"}, Sonnet: lipgloss.AdaptiveColor{Light: "#cc5522", Dark: "#ee8855"},
		Haiku: lipgloss.AdaptiveColor{Light: "#2255cc", Dark: "#77aaee"}, OK: lipgloss.AdaptiveColor{Light: "#228833", Dark: "#66cc88"},
		Bad: lipgloss.AdaptiveColor{Light: "#cc2222", Dark: "#ee6666"}, Selection: lipgloss.AdaptiveColor{Light: "#dddddd", Dark: "#333333"},
	})
	ended := fixtureState()
	ended.Ended = true
	wide := fixtureState() // ten agents at depth 1 wrap into a second line; a3 hangs below it
	for i := 0; i < 8; i++ {
		wide.Snap.Agents = append(wide.Snap.Agents, session.Agent{
			ID: fmt.Sprintf("w%d", i), Name: fmt.Sprintf("worker-%d", i), Model: "claude-haiku-4-5", Task: "scan a package",
			Start: wide.Now.Add(-time.Minute), Tool: "Read", Ctx: 10 + i,
		})
	}
	tests := []struct {
		name  string
		w, h  int
		st    State
		right bool // scrolled to the diagram's right edge
	}{
		{"full", 90, 46, fixtureState(), false},
		{"full_ended", 90, 46, ended, false},
		{"full_wide", 200, 50, wide, false},
		{"short", 90, 24, fixtureState(), false},
		{"narrow", 60, 20, fixtureState(), false},
		{"narrowest", 52, 15, fixtureState(), false},
		{"procs", 90, 24, withProcs(fixtureState()), true},
		{"procs_wide", 200, 50, withProcs(wide), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(theme)
			m.SetSize(tt.w, tt.h)
			m.SetState(tt.st)
			if tt.right {
				m.offX = far
			}
			got := ansi.Strip(m.View())
			path := filepath.Join("testdata", tt.name+".golden")
			if *update {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run go test -update to write it)", err)
			}
			if got != string(want) {
				t.Errorf("frame differs from %s (rerun with -update after checking it):\n%s", path, got)
			}
		})
	}
}

func TestViewFillsTheTerminal(t *testing.T) {
	for _, size := range [][2]int{{90, 46}, {120, 50}, {90, 24}, {60, 20}, {52, 15}, {40, 10}, {10, 3}, {0, 0}} {
		m := New(nil)
		m.SetSize(size[0], size[1])
		m.SetState(fixtureState())
		if w, h := lipgloss.Size(m.View()); size[1] > 0 && (w != size[0] || h != size[1]) {
			t.Errorf("view at %dx%d is %dx%d", size[0], size[1], w, h)
		}
	}
}

func TestSetSessionKeepsTheFirstSnapshotStill(t *testing.T) {
	m := New(nil)
	m.SetSize(90, 46)
	m.SetSession("fix~7", "", t0.Add(-time.Minute), t0)
	m.Apply(SnapshotEvent{session.Snapshot{
		Lead:   session.Lead{Model: "claude-opus-4-1"},
		Agents: []session.Agent{{ID: "a1", Name: "implementer", Model: "claude-opus-4-1", Start: t0, Ctx: -1}},
		Events: []session.LogEvent{{At: t0, Actor: "lead", Kind: session.LogSpawn, Text: "spawned implementer"}},
	}})
	if len(m.Packets()) != 0 {
		t.Errorf("first snapshot sent packets %v, want none: the agents were there before the view opened", m.Packets())
	}
	f := m.frame()
	if f.Title != "fix~7" || !f.Start.Equal(t0.Add(-time.Minute)) || !f.Now.Equal(t0) {
		t.Errorf("title %q, start %v, now %v; want the session's", f.Title, f.Start, f.Now)
	}
	if len(f.Log) != 1 || f.Log[0].Text != "spawned implementer" {
		t.Errorf("log %+v, want the agent's spawn", f.Log)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "AGENT VIEW · fix~7") || !strings.Contains(v, "1m00s") {
		t.Errorf("view lacks the title or the session's age:\n%s", v)
	}
}

func TestSetSessionTitlesTheHeader(t *testing.T) {
	tests := []struct {
		name, item string
		want       string
	}{
		{"the item's title", "Fix the cache", "AGENT VIEW · Fix the cache"},
		{"the session's name without one", "", "AGENT VIEW · fix~7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(nil)
			m.SetSize(90, 24)
			m.SetSession("fix~7", tt.item, t0.Add(-time.Minute), t0)
			v := strings.Split(ansi.Strip(m.View()), "\n")
			if got := strings.TrimSpace(v[0]); got != tt.want {
				t.Errorf("header = %q, want %q", got, tt.want)
			}
			if !strings.Contains(v[3]+v[4], "fix~7") {
				t.Errorf("session column lacks the session's name:\n%s", strings.Join(v, "\n"))
			}
		})
	}
}

// keyMsg is the key message Bubble Tea sends for k.
func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "ctrl+d":
		return tea.KeyMsg{Type: tea.KeyCtrlD}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

// withAgentsOf is the fixture with agents in place of its own.
func withAgentsOf(agents []session.Agent) State {
	st := fixtureState()
	st.Snap.Agents = agents
	return st
}

// deep is twelve agents over three depths: six of the lead, five of theirs and one below those.
func deep() []session.Agent {
	return many(6, "agent-06:agent-00", "agent-07:agent-01", "agent-08:agent-02", "agent-09:agent-03",
		"agent-10:agent-04", "agent-11:agent-06")
}

// scrolled is a w×h view of st after the keys.
func scrolled(st State, w, h int, keys ...string) Model {
	m := New(nil)
	m.SetSize(w, h)
	m.SetState(st)
	for _, k := range keys {
		m, _ = m.Update(keyMsg(k))
	}
	return m
}

func TestScrollKeys(t *testing.T) {
	// At 90×24 the viewport is 67×13 and the deep diagram 91×75 (the lead is centered over the 67 wide agent
	// tree, the tools box ends at column 91): it scrolls 62 lines and 24 columns. At 200×50 the diagram fits the width. At 52×15 the viewport is 29×4
	// and the diagram 75 wide (the lead and the tools box side by side): it scrolls 46 columns.
	tests := []struct {
		name   string
		w, h   int
		keys   []string
		wx, wy int
	}{
		{"j moves a line down", 90, 24, []string{"j"}, 0, 1},
		{"k moves a line up", 90, 24, []string{"j", "j", "k"}, 0, 1},
		{"k stops at the top", 90, 24, []string{"k"}, 0, 0},
		{"ctrl+d moves half the viewport down", 90, 24, []string{"ctrl+d"}, 0, 6},
		{"ctrl+u moves half the viewport up", 90, 24, []string{"ctrl+d", "ctrl+d", "ctrl+u"}, 0, 6},
		{"G goes to the bottom", 90, 24, []string{"G"}, 0, 62},
		{"j stops at the bottom", 90, 24, []string{"G", "j"}, 0, 62},
		{"ctrl+d stops at the bottom", 90, 24, []string{"G", "k", "ctrl+d"}, 0, 62},
		{"g goes to the top", 90, 24, []string{"G", "g"}, 0, 0},
		{"l does nothing when the diagram fits the width", 200, 50, []string{"l"}, 0, 0},
		{"h does nothing when the diagram fits the width", 200, 50, []string{"h"}, 0, 0},
		{"l stops at the right end at 90x24", 90, 24, strings.Split(strings.Repeat("l", 30), ""), 8, 0},
		{"l moves a column right", 52, 15, []string{"l"}, 1, 0},
		{"l stops at the right end", 52, 15, strings.Split(strings.Repeat("l", 70), ""), 46, 0},
		{"h moves a column left", 52, 15, []string{"l", "l", "h"}, 1, 0},
		{"h stops at the left end", 52, 15, []string{"h"}, 0, 0},
		{"arrows scroll like hjkl", 52, 15, []string{"right", "down", "down", "up"}, 1, 1},
		{"other keys do nothing", 90, 24, []string{"x", "enter"}, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(nil)
			m.SetSize(tt.w, tt.h)
			m.SetState(withAgentsOf(deep()))
			for _, k := range tt.keys {
				msg := keyMsg(k)
				switch k {
				case "right":
					msg = tea.KeyMsg{Type: tea.KeyRight}
				case "down":
					msg = tea.KeyMsg{Type: tea.KeyDown}
				case "up":
					msg = tea.KeyMsg{Type: tea.KeyUp}
				case "enter":
					msg = tea.KeyMsg{Type: tea.KeyEnter}
				}
				var cmd tea.Cmd
				if m, cmd = m.Update(msg); cmd != nil {
					t.Errorf("%s returned a command", k)
				}
			}
			if x, y := m.Offset(); x != tt.wx || y != tt.wy {
				t.Errorf("offset %d,%d; want %d,%d", x, y, tt.wx, tt.wy)
			}
		})
	}
}

func TestScrollOffsetSurvivesSnapshots(t *testing.T) {
	m := scrolled(withAgentsOf(deep()), 90, 19, "j", "j", "j")
	snap := m.st.Snap
	snap.Agents = append([]session.Agent(nil), snap.Agents...)
	snap.Agents[0].Tool = "Read"
	m.Apply(SnapshotEvent{snap})
	if x, y := m.Offset(); x != 0 || y != 3 {
		t.Errorf("after a snapshot: offset %d,%d; want 0,3", x, y)
	}
	m, _ = m.Update(keyMsg("G"))
	snap.Agents = snap.Agents[:1] // the diagram shrinks to 91×15: 7 lines past the 8 line viewport
	m.Apply(SnapshotEvent{snap})
	if x, y := m.Offset(); x != 0 || y != 7 {
		t.Errorf("after the diagram shrank: offset %d,%d; want 0,7, the new bottom", x, y)
	}
	if m, _ = m.Update(keyMsg("k")); func() int { _, y := m.Offset(); return y }() != 6 {
		t.Errorf("k after the shrink did not move up from the new bottom")
	}
}

// frameRows is the view's frame without styles, as rune rows.
func frameRows(m Model) [][]rune {
	var out [][]rune
	for _, ln := range strings.Split(ansi.Strip(m.View()), "\n") {
		out = append(out, []rune(ln))
	}
	return out
}

func TestScrollMovesOnlyTheDiagram(t *testing.T) {
	for _, size := range [][2]int{{90, 24}, {52, 15}} {
		w, h := size[0], size[1]
		t.Run(fmt.Sprintf("%dx%d", w, h), func(t *testing.T) {
			before := scrolled(withAgentsOf(deep()), w, h)
			after := scrolled(withAgentsOf(deep()), w, h, "j", "j", "l")
			l := before.layout
			dx, _ := after.Offset()
			a, b := frameRows(before), frameRows(after)
			v := l.View
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					inView := x >= v.X && x < v.X+v.W && y >= v.Y && y < v.Y+v.H
					indicator := y == l.Log.Y-1 && x >= v.X
					switch {
					case inView:
						// the diagram moved as one block: two lines up and dx columns left
						if y+2 < v.Y+v.H && x+dx < v.X+v.W && b[y][x] != a[y+2][x+dx] {
							t.Fatalf("viewport cell %d,%d = %q, want %q from %d,%d before", x, y, b[y][x], a[y+2][x+dx], x+dx, y+2)
						}
					case !indicator && b[y][x] != a[y][x]:
						t.Fatalf("pinned cell %d,%d changed from %q to %q:\n%s", x, y, a[y][x], b[y][x], string(b[y]))
					}
				}
			}
			for _, pinned := range []string{"AGENT VIEW", "■ opus", "┏━ session", "session log", "● live"} {
				if !strings.Contains(ansi.Strip(after.View()), pinned) {
					t.Errorf("scrolled frame lacks %q", pinned)
				}
			}
		})
	}
}

func TestScrollReachesEveryAgent(t *testing.T) {
	sets := map[string][]session.Agent{"1": many(1), "5": many(5), "7": many(7), "12": many(12), "12 over 3 depths": deep()}
	for _, size := range sizes {
		for name, agents := range sets {
			t.Run(fmt.Sprintf("%s at %dx%d", name, size[0], size[1]), func(t *testing.T) {
				m := scrolled(withAgentsOf(agents), size[0], size[1])
				var seen strings.Builder
				for {
					seen.WriteString(ansi.Strip(m.View()))
					_, y := m.Offset()
					if m, _ = m.Update(keyMsg("j")); func() int { _, ny := m.Offset(); return ny }() == y {
						break
					}
				}
				for _, a := range agents {
					if !strings.Contains(seen.String(), "━ "+a.Name+" ━") {
						t.Errorf("no frame scrolled to shows the box of %s", a.Name)
					}
				}
				if strings.Contains(seen.String(), "more") {
					t.Error("a frame shows a +n more box")
				}
			})
		}
	}
}

func TestScrollIndicator(t *testing.T) {
	tests := []struct {
		name    string
		w, h    int
		agents  []session.Agent
		keys    []string
		want    string
		without []string
	}{
		{"nothing to scroll, only the focus", 200, 50, fixtureState().Snap.Agents, nil, "diagram ", []string{"lines ", "cols "}},
		{"nothing to scroll, not focused: nothing", 200, 50, fixtureState().Snap.Agents, []string{"tab"}, "", []string{"diagram", "lines ", "cols "}},
		{"lines shown of the diagram's", 90, 24, deep(), []string{"j"}, "diagram  lines 2-14/75  cols 1-67/75", nil},
		{"columns too on a narrow terminal", 52, 15, deep(), []string{"l"}, "diagram  lines 1-4/125  cols 2-30/75", nil},
		{"not focused: the position without the name", 90, 24, deep(), []string{"tab"}, "lines 1-13/75  cols 1-67/75", []string{"diagram"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := scrolled(withAgentsOf(tt.agents), tt.w, tt.h, tt.keys...)
			row := string(frameRows(m)[m.layout.Log.Y-1])
			if !strings.Contains(row, tt.want) {
				t.Errorf("row above the log = %q, want it to hold %q", row, tt.want)
			}
			for _, w := range tt.without {
				if strings.Contains(row, w) {
					t.Errorf("row above the log = %q has %q", row, w)
				}
			}
		})
	}
}

// numberedLog is n log lines numbered from 1, a second apart.
func numberedLog(n int) []LogLine {
	var out []LogLine
	for i := 1; i <= n; i++ {
		out = append(out, LogLine{At: t0.Add(time.Duration(i) * time.Second), Actor: "lead", Text: fmt.Sprintf("line %d", i)})
	}
	return out
}

// logFrame is the session log box of m's frame: the top border and the shown lines without their borders.
func logFrame(m Model) []string {
	rows := frameRows(m)
	l := m.layout.Log
	out := []string{string(rows[l.Y])}
	for y := l.Y + 1; y < l.Y+l.H-1; y++ {
		out = append(out, strings.TrimSpace(strings.Trim(string(rows[y]), "│")))
	}
	return out
}

// appendLog applies a snapshot that logs one more line, numbered after the log's last.
func appendLog(m *Model) {
	n := len(m.st.Log) + 1
	snap := m.st.Snap
	snap.Events = []session.LogEvent{{At: t0.Add(time.Duration(n) * time.Second), Actor: "lead", Text: fmt.Sprintf("line %d", n)}}
	m.Apply(SnapshotEvent{snap})
}

func TestLogBox(t *testing.T) {
	st := fixtureState()
	st.Log = numberedLog(2)
	m := scrolled(st, 90, 24)
	l := m.layout.Log
	rows := frameRows(m)
	if l.H != 6 {
		t.Fatalf("log box is %d rows high, want 6: 4 lines in a border", l.H)
	}
	corners := string([]rune{rows[l.Y][0], rows[l.Y][l.W-1], rows[l.Y+l.H-1][0], rows[l.Y+l.H-1][l.W-1]})
	if corners != "┌┐└┘" || !strings.HasPrefix(string(rows[l.Y]), "┌─ session log ─") {
		t.Errorf("log box top %q, corners %q; want a square box titled session log", string(rows[l.Y]), corners)
	}
	if got := logFrame(m); strings.Contains(got[0], "/") || !strings.HasSuffix(got[1], "line 1") ||
		!strings.HasSuffix(got[2], "line 2") || got[3] != "" || got[4] != "" {
		t.Errorf("a short log = %q, want its two lines at the top and no position", got)
	}
}

func TestLogLineRoles(t *testing.T) {
	s := State{Log: []LogLine{
		{At: t0, Actor: "reviewer", Model: "claude-sonnet-4-5", Text: "Bash go test", Bad: true},
		{At: t0, Actor: "lead", Model: "claude-opus-4-1", Text: "Read x.go"},
	}}
	c := NewCanvas(60, 4, nil)
	s.drawLog(c, Rect{0, 0, 60, 4})
	// time at 2, actor at 11 (8 wide: "reviewer"), text at 20
	tests := []struct {
		name string
		x, y int
		want Role
	}{
		{"time dim", 2, 1, RoleDim},
		{"actor in its family color", 11, 1, RoleSonnet},
		{"failed call in the bad color", 20, 1, RoleBad},
		{"lead in its family color", 11, 2, RoleOpus},
		{"call in the text color", 20, 2, RoleText},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.At(tt.x, tt.y).Role; got != tt.want {
				t.Errorf("cell %d,%d (%q) role %d, want %d", tt.x, tt.y, c.At(tt.x, tt.y).R, got, tt.want)
			}
		})
	}
}

func TestFocusCycles(t *testing.T) {
	tests := []struct {
		keys []string
		want area
	}{
		{nil, areaDiagram},
		{[]string{"tab"}, areaLog},
		{[]string{"tab", "tab"}, areaSession},
		{[]string{"tab", "tab", "tab"}, areaDiagram},
		{[]string{"shift+tab"}, areaSession},
		{[]string{"shift+tab", "shift+tab"}, areaLog},
		{[]string{"shift+tab", "shift+tab", "shift+tab"}, areaDiagram},
		{[]string{"tab", "shift+tab"}, areaDiagram},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.keys, ","), func(t *testing.T) {
			m := New(nil)
			m.SetSize(90, 24)
			m.SetState(fixtureState())
			for _, k := range tt.keys {
				msg := tea.KeyMsg{Type: tea.KeyTab}
				if k == "shift+tab" {
					msg = tea.KeyMsg{Type: tea.KeyShiftTab}
				}
				m, _ = m.Update(msg)
			}
			if m.focus != tt.want {
				t.Errorf("focus %d, want %d", m.focus, tt.want)
			}
		})
	}
}

func TestFocusedAreaIsHighlighted(t *testing.T) {
	theme := NewTheme(NeonPalette())
	tests := []struct {
		name string
		keys []string
		want map[string]bool // area → accent
	}{
		{"diagram on open", nil, map[string]bool{"session": false, "diagram": true, "log": false}},
		{"log", []string{"tab"}, map[string]bool{"session": false, "diagram": false, "log": true}},
		{"session", []string{"tab", "tab"}, map[string]bool{"session": true, "diagram": false, "log": false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(theme)
			m.SetSize(90, 24)
			m.SetState(withAgentsOf(deep()))
			for range tt.keys {
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
			}
			m.View()
			l := m.layout
			pos := m.canvas.At(l.Frame.W-2, l.Log.Y-1) // the end of the diagram's position
			got := map[string]bool{
				"session": m.canvas.At(l.Session.X, l.Session.Y).Role == RoleAccent,
				"diagram": pos.Role == RoleAccent,
				"log":     m.canvas.At(l.Log.X, l.Log.Y).Role == RoleAccent,
			}
			for a, want := range tt.want {
				if got[a] != want {
					t.Errorf("%s highlighted %v, want %v", a, got[a], want)
				}
			}
		})
	}
}

func TestOnlyTheFocusedAreaScrolls(t *testing.T) {
	st := withAgentsOf(deep())
	st.Log = numberedLog(10)
	tests := []struct {
		name      string
		keys      []string
		wantDiagY int    // the diagram's line offset
		wantLog   string // the log's first shown line
		wantSess  string // the start of the session column's first shown row
	}{
		{"diagram", []string{"j", "j"}, 2, "line 7", "issue-196"},
		{"log", []string{"tab", "k", "k"}, 0, "line 5", "issue-196"},
		{"session", []string{"tab", "tab", "j"}, 0, "line 7", "ctx"},
		{"back on the diagram", []string{"tab", "k", "tab", "tab", "j"}, 1, "line 6", "issue-196"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := scrolled(st, 90, 24, tt.keys...)
			r := m.layout.Session
			sess := strings.TrimLeft(string(frameRows(m)[r.Y+1][:r.W]), "┃ ")
			if _, y := m.Offset(); y != tt.wantDiagY || !strings.HasSuffix(logFrame(m)[1], " "+tt.wantLog) ||
				!strings.HasPrefix(sess, tt.wantSess) {
				t.Errorf("diagram line %d, log %q, session row %q; want %d, %q, %q",
					y, logFrame(m)[1], sess, tt.wantDiagY, tt.wantLog, tt.wantSess)
			}
		})
	}
}

func TestLogScrolling(t *testing.T) {
	tests := []struct {
		name      string
		keys      []string
		appends   int
		wantFirst string // first shown line
		wantPos   string
	}{
		{"follows the newest", nil, 0, "line 7", " 10/10 "},
		{"follows while lines append", nil, 3, "line 10", " 13/13 "},
		{"k scrolls up a line", []string{"k"}, 0, "line 6", " 9/10 "},
		{"held while lines append", []string{"k"}, 3, "line 6", " 9/13 "},
		{"ctrl+u half the box", []string{"ctrl+u"}, 0, "line 5", " 8/10 "},
		{"g to the oldest", []string{"g"}, 2, "line 1", " 4/12 "},
		{"k stops at the oldest", []string{"g", "k"}, 0, "line 1", " 4/10 "},
		{"G follows again", []string{"g", "G"}, 2, "line 9", " 12/12 "},
		{"j back to the bottom follows again", []string{"k", "j"}, 2, "line 9", " 12/12 "},
		{"ctrl+d down from the top", []string{"g", "ctrl+d"}, 0, "line 3", " 6/10 "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := fixtureState()
			st.Log = numberedLog(10)
			m := scrolled(st, 90, 24, append([]string{"tab"}, tt.keys...)...)
			for i := 0; i < tt.appends; i++ {
				appendLog(&m)
			}
			got := logFrame(m)
			if !strings.HasSuffix(got[1], " "+tt.wantFirst) || !strings.Contains(got[0], tt.wantPos) {
				t.Errorf("log box = %q, want it to start at %q and show %q", got, tt.wantFirst, tt.wantPos)
			}
		})
	}
}

func TestHeldLogStaysPutWhenTheOldestDrop(t *testing.T) {
	st := fixtureState()
	st.Log = numberedLog(maxLog)
	m := scrolled(st, 90, 24, "tab", "g", "j", "j") // lines 3-6
	appendLog(&m)                                   // line 1 dropped
	if got := logFrame(m); !strings.HasSuffix(got[1], " line 3") {
		t.Errorf("log box = %q, want it still at line 3", got)
	}
}

func TestEndedLogScrolls(t *testing.T) {
	st := fixtureState()
	st.Ended = true
	st.Log = numberedLog(10)
	m := scrolled(st, 90, 24, "tab", "g")
	if got := logFrame(m); !strings.HasSuffix(got[1], " line 1") {
		t.Errorf("ended log box = %q, want it scrolled to line 1", got)
	}
}

func TestSessionColumnScrolls(t *testing.T) {
	tests := []struct {
		name  string
		h     int
		keys  []string
		first string // the column's first row once scrolled
	}{
		{"taller than the column: j scrolls a row", 24, []string{"j"}, "ctx"},
		{"G to the last rows", 24, []string{"G"}, "state.go"},
		{"k stops at the top", 24, []string{"k"}, "issue-196"},
		{"k after G moves up from the bottom", 24, []string{"G", "G", "k"}, "render_panels.go"},
		{"fits the column: keys do nothing", 50, []string{"j", "G"}, "issue-196"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := scrolled(fixtureState(), 90, tt.h, append([]string{"tab", "tab"}, tt.keys...)...)
			r := m.layout.Session
			if row := string(frameRows(m)[r.Y+1][:r.W]); !strings.HasPrefix(strings.TrimLeft(row, "┃ "), strings.TrimSpace(tt.first)) {
				t.Errorf("first row %q, want it to start with %q", row, tt.first)
			}
		})
	}
}

func TestLogPositionLeftOutWhenItDoesNotFit(t *testing.T) {
	s := State{Log: numberedLog(10)}
	for _, tt := range []struct {
		w    int
		want bool
	}{{40, true}, {22, false}} {
		c := NewCanvas(tt.w, 6, nil)
		s.drawLog(c, Rect{0, 0, tt.w, 6})
		top := rows(c)[0]
		if strings.Contains(top, "10/10") != tt.want || !strings.Contains(top, "session log") {
			t.Errorf("width %d: top border %q, want the title and the position shown %v", tt.w, top, tt.want)
		}
	}
}

// procsBox returns the rows of the processes box in frame v, its title row first, cut at the box's left border; nil
// when the frame has no processes box.
func procsBox(v string) []string {
	lines := strings.Split(v, "\n")
	for i, ln := range lines {
		r := []rune(ln)
		for x := range r {
			if strings.HasPrefix(string(r[x:]), "┏━ processes ") {
				var rows []string
				for _, l := range lines[i:min(i+7, len(lines))] { // border, five processes, border
					rows = append(rows, string([]rune(l)[x:]))
				}
				return rows
			}
		}
	}
	return nil
}

func TestProcessesBox(t *testing.T) {
	first := fixtureState()
	first.Procs = []session.Proc{ // a first sample: no CPU share yet
		{PID: 10, PPID: 1, RSS: 2 << 20, Command: "-zsh"},
		{PID: 11, PPID: 10, RSS: 300 << 20, Command: "claude"},
	}
	tests := []struct {
		name string
		st   State
		want [][]string // each row's fields, in order: agent, cpu, memory, command
	}{
		{"top five by cpu, each under the agent running it", withProcs(fixtureState()), [][]string{
			{"implementer", "180%", "1.5G", "go test ./..."},
			{"implementer", "96%", "210M", "compile -o x.a"},
			{"lead", "35%", "420M", "claude --model opus"},
			{"lead", "2%", "80M", "node mcp-server.js"},
			{"implementer", "0%", "3M", "zsh -c -l eval"},
		}},
		{"first sample has no cpu share", first, [][]string{
			{"lead", "n/a", "2M", "-zsh"},
			{"lead", "n/a", "300M", "claude"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(nil)
			m.SetSize(90, 46)
			m.SetState(tt.st)
			m.offX = far
			box := procsBox(m.View())
			if box == nil {
				t.Fatalf("no processes box:\n%s", m.View())
			}
			for i, fields := range tt.want {
				row := box[1+i]
				at := 0
				for _, f := range fields {
					k := strings.Index(row[at:], f)
					if k < 0 {
						t.Fatalf("row %d %q lacks %q after column %d; want fields %q", i, row, f, at, fields)
					}
					at += k + len(f)
				}
			}
			if rest := box[1+len(tt.want)]; len(tt.want) < 5 && strings.TrimSpace(strings.Trim(rest, "┃ ")) != "" {
				t.Errorf("row after the processes is %q, want it empty", rest)
			}
		})
	}
}

func TestProcessesBoxIsLeftOutWithoutData(t *testing.T) {
	m := New(nil)
	m.SetSize(200, 50)
	m.SetState(withProcs(fixtureState()))
	if procsBox(scrolledRight(m)) == nil {
		t.Fatal("no processes box with processes")
	}
	m.Apply(ProcsEvent{})
	if v := scrolledRight(m); procsBox(v) != nil {
		t.Errorf("an empty sample still shows the processes box:\n%s", v)
	}
	m.Apply(ProcsEvent{Procs: withProcs(State{}).Procs})
	m.Apply(EndEvent{})
	if v := scrolledRight(m); procsBox(v) != nil {
		t.Errorf("an ended session still shows the processes box:\n%s", v)
	}
}

// scrolledRight is m's frame scrolled to the diagram's right edge.
func scrolledRight(m Model) string {
	m.offX = far
	return m.View()
}
