package flowview

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
			},
			Agents: []session.Agent{
				{ID: "a1", Name: "implementer", Model: "claude-opus-4-1", Task: "slice 2 of #196", Start: ago(9 * time.Minute), Tool: "Bash", Ctx: 42},
				{ID: "a2", Name: "triager", Model: "claude-haiku-4-5", Task: "triage #201", Status: session.AgentDone, Start: ago(11 * time.Minute), End: ago(10 * time.Minute), Ctx: 7},
				{ID: "a3", Parent: "a1", Name: "code-reviewer", Model: "claude-sonnet-4-5", Task: "review the diff", Start: ago(2 * time.Minute), Tool: "Grep", Ctx: -1},
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
		{"compact", 70, 30, fixtureState()},
		{"too_small", 50, 20, fixtureState()},
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
	for _, size := range [][2]int{{90, 46}, {120, 50}, {70, 30}, {50, 20}, {0, 0}} {
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
