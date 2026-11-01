package flowview

import (
	"fmt"
	"testing"

	"github.com/chriha/lanes/internal/session"
)

// agentsOf builds agents from "id:parent" pairs.
func agentsOf(specs ...string) []session.Agent {
	out := make([]session.Agent, len(specs))
	for i, s := range specs {
		var id, parent string
		for j := range s {
			if s[j] == ':' {
				id, parent = s[:j], s[j+1:]
			}
		}
		if id == "" {
			id = s
		}
		out[i] = session.Agent{ID: id, Parent: parent, Name: id}
	}
	return out
}

func TestLayoutTree(t *testing.T) {
	area := Rect{0, 0, 90, 21} // 4 boxes of 21 per line, 3 lines
	box := Rect{W: 21, H: 5}
	// node renders a node as "name@row^parent", "+n@row^parent" for a "+n more" box; parent "-" is the lead.
	type want []string
	tests := []struct {
		name   string
		agents []session.Agent
		want   want
	}{
		{"none", nil, nil},
		{"one agent hangs off the lead", agentsOf("a"), want{"a@0^-"}},
		{"three agents in one row", agentsOf("a", "b", "c"), want{"a@0^-", "b@0^-", "c@0^-"}},
		{
			"seven agents wrap into a second line of the same depth",
			agentsOf("a", "b", "c", "d", "e", "f", "g"),
			want{"a@0^-", "b@0^-", "c@0^-", "d@0^-", "e@1^-", "f@1^-", "g@1^-"},
		},
		{
			"a row wraps into at most two lines before +n more",
			agentsOf("a", "b", "c", "d", "e", "f", "g", "h", "i", "j"),
			want{"a@0^-", "b@0^-", "c@0^-", "d@0^-", "e@1^-", "f@1^-", "g@1^-", "+3@1^-"},
		},
		{
			"a wrapped row leaves a line for the next depth",
			agentsOf("a", "b", "c", "d", "e", "f", "g", "h", "i", "x:a"),
			want{"a@0^-", "b@0^-", "c@0^-", "d@0^-", "e@1^-", "f@1^-", "g@1^-", "+2@1^-", "x@2^a"},
		},
		{
			"depth 3: each depth on its own row, linked to its parent",
			agentsOf("a", "b", "a1:a", "b1:b", "a11:a1"),
			want{"a@0^-", "b@0^-", "a1@1^a", "b1@1^b", "a11@2^a1"},
		},
		{
			"unknown parent hangs off the lead",
			agentsOf("x:gone"),
			want{"x@0^-"},
		},
		{
			"a child of a folded agent hangs off the +n more box",
			agentsOf("a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "j1:j"),
			want{"a@0^-", "b@0^-", "c@0^-", "d@0^-", "e@1^-", "f@1^-", "g@1^-", "+3@1^-", "j1@2^+3"},
		},
		{
			"agents deeper than the last row fold into its +n more box",
			agentsOf("a", "a1:a", "a2:a1", "a3:a2"),
			want{"a@0^-", "a1@1^a", "a2@2^a1", "+1@2^a1"},
		},
		{
			"a parent cycle does not hang",
			agentsOf("p:q", "q:p"),
			want{"q@0^-", "p@1^q"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := layoutTree(tt.agents, area, box, colGap)
			label := func(i int) string {
				if i < 0 {
					return "-"
				}
				if n := nodes[i]; n.Agent >= 0 {
					return tt.agents[n.Agent].ID
				}
				return fmt.Sprintf("+%d", nodes[i].More)
			}
			var got []string
			for i, n := range nodes {
				if n.Rect.W != box.W || n.Rect.H != box.H || (n.Rect.Y-area.Y)%(box.H+rowGap) != 0 {
					t.Fatalf("node %d rect %+v is off the row grid", i, n.Rect)
				}
				if n.Rect.X < area.X || n.Rect.X+n.Rect.W > area.X+area.W {
					t.Fatalf("node %d rect %+v leaves the area", i, n.Rect)
				}
				if i > 0 && nodes[i-1].Rect.Y == n.Rect.Y && nodes[i-1].Rect.X+box.W+colGap != n.Rect.X {
					t.Fatalf("nodes %d and %d are not side by side", i-1, i)
				}
				got = append(got, fmt.Sprintf("%s@%d^%s", label(i), (n.Rect.Y-area.Y)/(box.H+rowGap), label(n.Parent)))
			}
			if fmt.Sprint(got) != fmt.Sprint([]string(tt.want)) {
				t.Errorf("nodes = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLayoutTreeFoldsDeeperAgentsIntoAFullLastRow(t *testing.T) {
	// 2 slots, 2 rows: the last row holds a1 and b1, and x is one row deeper.
	agents := agentsOf("a", "a1:a", "b1:a", "x:a1")
	nodes := layoutTree(agents, Rect{0, 0, 44, 13}, Rect{W: 21, H: 5}, colGap)
	if len(nodes) != 3 || nodes[1].Agent != 1 || nodes[2].Agent != -1 || nodes[2].More != 2 || nodes[2].Parent != 0 {
		t.Fatalf("nodes = %+v, want a, a1 and a +2 more box under a", nodes)
	}
}

func TestLayoutTreeCentersRows(t *testing.T) {
	nodes := layoutTree(agentsOf("a"), Rect{10, 0, 41, 5}, Rect{W: 21, H: 5}, colGap)
	if len(nodes) != 1 || nodes[0].Rect.X != 20 {
		t.Fatalf("nodes = %+v, want one box at x 20", nodes)
	}
}

func TestNewLayout(t *testing.T) {
	tests := []struct {
		w, h  int
		mode  Mode
		frame Rect
	}{
		{90, 46, ModeFull, Rect{0, 0, 90, 46}},
		{120, 60, ModeFull, Rect{0, 0, 120, 60}},
		{200, 50, ModeFull, Rect{0, 0, 200, 50}},
		{89, 46, ModeCompact, Rect{0, 0, 89, 46}},
		{120, 30, ModeCompact, Rect{15, 0, 90, 30}},
		{60, 24, ModeCompact, Rect{0, 0, 60, 24}},
		{59, 30, ModeTooSmall, Rect{0, 0, 59, 30}},
		{70, 23, ModeTooSmall, Rect{0, 0, 70, 23}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%dx%d", tt.w, tt.h), func(t *testing.T) {
			l := NewLayout(tt.w, tt.h)
			if l.Mode != tt.mode || l.Frame != tt.frame {
				t.Fatalf("mode %v frame %+v, want %v %+v", l.Mode, l.Frame, tt.mode, tt.frame)
			}
			if l.Mode == ModeTooSmall {
				return
			}
			if l.Mode == ModeFull {
				if l.Box.W < 28 || l.Box.W > 30 {
					t.Errorf("box width %d, want 28 to 30", l.Box.W)
				}
				if l.Tree.X != 0 || l.Tree.W != tt.w || l.Log.W != tt.w {
					t.Errorf("tree %+v and log %+v do not span the terminal width %d", l.Tree, l.Log, tt.w)
				}
				if l.Lead.X < l.Session.X+l.Session.W {
					t.Errorf("lead %+v overlaps the session column %+v", l.Lead, l.Session)
				}
			}
			if l.Tree.H < l.Box.H {
				t.Errorf("tree area %+v holds no row of %d", l.Tree, l.Box.H)
			}
			if l.Tree.Y+l.Tree.H >= l.Log.Y || l.Log.Y+l.Log.H > l.Status {
				t.Errorf("tree %+v, log %+v and status %d overlap", l.Tree, l.Log, l.Status)
			}
		})
	}
}

// depthsOf returns each agent's spawn depth, 1 for the lead's children.
func depthsOf(agents []session.Agent) []int {
	byID := map[string]session.Agent{}
	for _, a := range agents {
		byID[a.ID] = a
	}
	out := make([]int, len(agents))
	for i, a := range agents {
		for d := 1; ; d++ {
			p, ok := byID[a.Parent]
			if !ok {
				out[i] = d
				break
			}
			a = p
		}
	}
	return out
}

// TestFullLayoutFillsTheWidth lays out agent trees at both ends of the full layout and checks the boxes, the
// lines they take and that every box is connected to its parent without a connector crossing a box.
func TestFullLayoutFillsTheWidth(t *testing.T) {
	many := func(n int, extra ...string) []session.Agent {
		specs := make([]string, n)
		for i := range specs {
			specs[i] = fmt.Sprintf("agent-%02d", i)
		}
		return agentsOf(append(specs, extra...)...)
	}
	depth3 := agentsOf("a", "b", "a1:a", "b1:b", "a11:a1")
	tests := []struct {
		name   string
		w, h   int
		agents []session.Agent
		lines  string // boxes per line, top down
		more   int    // agents folded into "+n more"
	}{
		{"1 agent at 90", 90, 46, many(1), "[1]", 0},
		{"5 agents at 90", 90, 46, many(5), "[3 2]", 0},
		{"7 agents at 90", 90, 46, many(7), "[3 3]", 2},
		{"12 agents at 90", 90, 46, many(12), "[3 3]", 7},
		{"depth 3 at 90", 90, 46, depth3, "[2 2 1]", 0},
		{"a child under a wrapped row at 90", 90, 46, many(7, "x:agent-00"), "[3 3 1]", 2},
		{"1 agent at 200", 200, 50, many(1), "[1]", 0},
		{"5 agents at 200", 200, 50, many(5), "[5]", 0},
		{"7 agents at 200", 200, 50, many(7), "[6 1]", 0},
		{"12 agents at 200", 200, 50, many(12), "[6 6]", 0},
		{"depth 3 at 200", 200, 50, depth3, "[2 2 1]", 0},
		{"a child under a wrapped row at 200", 200, 50, many(12, "x:agent-00"), "[6 6 1]", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewLayout(tt.w, tt.h)
			nodes := layoutTree(tt.agents, l.Tree, l.Box, l.Gap)
			depth := depthsOf(tt.agents)
			var counts []int
			lineDepths := map[int]map[int]bool{} // depth -> lines (Y) it takes
			more, lastY := 0, -1
			for _, n := range nodes {
				if n.Rect.W < 28 {
					t.Errorf("box %+v is narrower than 28", n.Rect)
				}
				if n.Rect.Y != lastY {
					counts = append(counts, 0)
					lastY = n.Rect.Y
				}
				counts[len(counts)-1]++
				if n.Agent < 0 {
					more += n.More
					continue
				}
				d := depth[n.Agent]
				if lineDepths[d] == nil {
					lineDepths[d] = map[int]bool{}
				}
				lineDepths[d][n.Rect.Y] = true
			}
			if got := fmt.Sprint(counts); got != tt.lines || more != tt.more {
				t.Errorf("boxes per line %s, +%d more; want %s, +%d more", got, more, tt.lines, tt.more)
			}
			for d, ys := range lineDepths {
				if len(ys) > 2 {
					t.Errorf("depth %d takes %d lines, want at most 2", d, len(ys))
				}
			}
			for i, p := range treePaths(nodes, l.Tools, l.Tree) {
				n := nodes[i]
				from := Point{l.Tools.X + l.Tools.W/2, l.Tools.Y + l.Tools.H}
				if n.Parent >= 0 {
					pr := nodes[n.Parent].Rect
					from = Point{pr.X + pr.W/2, pr.Y + pr.H}
				}
				if to := (Point{n.Rect.X + n.Rect.W/2, n.Rect.Y - 1}); p[0] != from || p[len(p)-1] != to {
					t.Fatalf("connector of node %d runs %v to %v, want %v to %v", i, p[0], p[len(p)-1], from, to)
				}
				for k, pt := range p {
					if k > 0 {
						if dx, dy := pt.X-p[k-1].X, pt.Y-p[k-1].Y; dx*dx+dy*dy != 1 {
							t.Fatalf("connector of node %d jumps from %v to %v", i, p[k-1], pt)
						}
					}
					if pt.X < l.Tree.X || pt.X >= l.Tree.X+l.Tree.W {
						t.Fatalf("connector of node %d leaves the tree area at %v", i, pt)
					}
					for _, o := range nodes {
						if r := o.Rect; pt.X >= r.X && pt.X < r.X+r.W && pt.Y >= r.Y && pt.Y < r.Y+r.H {
							t.Fatalf("connector of node %d crosses box %+v at %v", i, r, pt)
						}
					}
				}
			}
		})
	}
}
