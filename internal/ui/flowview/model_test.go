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
				{ID: "a1", Name: "implementer", Model: "claude-opus-4-1", Task: "slice 2 of #196", Start: ago(9 * time.Minute), Tool: "Bash", ToolArg: "go test ./...", Ctx: 42, Effort: "high"},
				{ID: "a2", Name: "triager", Model: "claude-haiku-4-5", Task: "triage #201", Status: session.AgentDone, Start: ago(11 * time.Minute), End: ago(10 * time.Minute), Ctx: 7, Effort: "low"},
				{ID: "a3", Parent: "a1", Name: "code-reviewer", Model: "claude-sonnet-4-5", Task: "review the diff", Start: ago(2 * time.Minute), Tool: "Grep", ToolArg: "drawAgent", Ctx: -1, Effort: "medium"},
				{ID: "a4", Parent: "a3", Name: "validator", Model: "claude-sonnet-4-5", Task: "validate a finding", Status: session.AgentError, Start: ago(90 * time.Second), End: ago(30 * time.Second), Ctx: 12},
			},
		},
		Log: []LogLine{
			{At: ago(5 * time.Minute), Actor: "lead", Model: "claude-opus-4-1", Text: "spawned implementer"},
			{At: ago(2 * time.Minute), Actor: "implementer", Model: "claude-opus-4-1", Text: "spawned code-reviewer"},
			{At: ago(90 * time.Second), Actor: "code-reviewer", Model: "claude-sonnet-4-5", Text: "spawned validator"},
			{At: ago(30 * time.Second), Actor: "validator", Model: "claude-sonnet-4-5", Text: "failed"},
		},
	}
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
		name string
		w, h int
		st   State
	}{
		{"full", 90, 46, fixtureState()},
		{"full_ended", 90, 46, ended},
		{"full_wide", 200, 50, wide},
		{"short", 90, 24, fixtureState()},
		{"narrow", 60, 20, fixtureState()},
		{"narrowest", 52, 15, fixtureState()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(theme)
			m.SetSize(tt.w, tt.h)
			m.SetState(tt.st)
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
	m.SetSession("fix~7", t0.Add(-time.Minute), t0)
	m.Apply(SnapshotEvent{session.Snapshot{
		Lead:   session.Lead{Model: "claude-opus-4-1"},
		Agents: []session.Agent{{ID: "a1", Name: "implementer", Model: "claude-opus-4-1", Start: t0, Ctx: -1}},
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
	// At 90×24 the viewport is 67×15 and the deep diagram 67×69: it scrolls 54 lines and no column. At 52×15 the
	// viewport is 29×6 and the diagram 36 wide: it scrolls 7 columns.
	tests := []struct {
		name   string
		w, h   int
		keys   []string
		wx, wy int
	}{
		{"j moves a line down", 90, 24, []string{"j"}, 0, 1},
		{"k moves a line up", 90, 24, []string{"j", "j", "k"}, 0, 1},
		{"k stops at the top", 90, 24, []string{"k"}, 0, 0},
		{"ctrl+d moves half the viewport down", 90, 24, []string{"ctrl+d"}, 0, 7},
		{"ctrl+u moves half the viewport up", 90, 24, []string{"ctrl+d", "ctrl+d", "ctrl+u"}, 0, 7},
		{"G goes to the bottom", 90, 24, []string{"G"}, 0, 54},
		{"j stops at the bottom", 90, 24, []string{"G", "j"}, 0, 54},
		{"ctrl+d stops at the bottom", 90, 24, []string{"G", "k", "ctrl+d"}, 0, 54},
		{"g goes to the top", 90, 24, []string{"G", "g"}, 0, 0},
		{"l does nothing when the diagram fits the width", 90, 24, []string{"l"}, 0, 0},
		{"h does nothing when the diagram fits the width", 90, 24, []string{"h"}, 0, 0},
		{"l moves a column right", 52, 15, []string{"l"}, 1, 0},
		{"l stops at the right end", 52, 15, []string{"l", "l", "l", "l", "l", "l", "l", "l", "l", "l"}, 7, 0},
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
	m := scrolled(withAgentsOf(deep()), 90, 24, "j", "j", "j")
	snap := m.st.Snap
	snap.Agents = append([]session.Agent(nil), snap.Agents...)
	snap.Agents[0].Tool = "Read"
	m.Apply(SnapshotEvent{snap})
	if x, y := m.Offset(); x != 0 || y != 3 {
		t.Errorf("after a snapshot: offset %d,%d; want 0,3", x, y)
	}
	m, _ = m.Update(keyMsg("G"))
	snap.Agents = snap.Agents[:1] // the diagram shrinks to 67×21: 6 lines past the viewport
	m.Apply(SnapshotEvent{snap})
	if x, y := m.Offset(); x != 0 || y != 6 {
		t.Errorf("after the diagram shrank: offset %d,%d; want 0,6, the new bottom", x, y)
	}
	if m, _ = m.Update(keyMsg("k")); func() int { _, y := m.Offset(); return y }() != 5 {
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
			for _, pinned := range []string{"AGENT VIEW", "■ opus", "╭─ session", "session log", "● live"} {
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
					if !strings.Contains(seen.String(), "─ "+a.Name+" ─") {
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
		{"nothing to scroll, no indicator", 200, 50, fixtureState().Snap.Agents, nil, "", []string{"lines ", "cols "}},
		{"lines shown of the diagram's", 90, 24, deep(), []string{"j"}, "lines 2-16/69", []string{"cols "}},
		{"columns too on a narrow terminal", 52, 15, deep(), []string{"l"}, "lines 1-6/109  cols 2-30/36", nil},
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
