package flowview

import (
	"fmt"
	"strings"
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
	area := Rect{0, 0, 90, 0} // 4 boxes of 21 per line
	box := Rect{W: 21, H: 5}
	// node renders a node as "name@line^parent"; parent "-" is the lead.
	type want []string
	tests := []struct {
		name   string
		agents []session.Agent
		want   want
	}{
		{"none", nil, nil},
		{"one agent hangs off the lead", agentsOf("a"), want{"a@0^-"}},
		{"three agents in one line", agentsOf("a", "b", "c"), want{"a@0^-", "b@0^-", "c@0^-"}},
		{
			"seven agents wrap into a second line of the same depth",
			agentsOf("a", "b", "c", "d", "e", "f", "g"),
			want{"a@0^-", "b@0^-", "c@0^-", "d@0^-", "e@1^-", "f@1^-", "g@1^-"},
		},
		{
			"a depth wraps into as many lines as it needs",
			agentsOf("a", "b", "c", "d", "e", "f", "g", "h", "i", "j"),
			want{"a@0^-", "b@0^-", "c@0^-", "d@0^-", "e@1^-", "f@1^-", "g@1^-", "h@1^-", "i@2^-", "j@2^-"},
		},
		{
			"the next depth goes below a wrapped one",
			agentsOf("a", "b", "c", "d", "e", "f", "g", "h", "i", "x:a"),
			want{"a@0^-", "b@0^-", "c@0^-", "d@0^-", "e@1^-", "f@1^-", "g@1^-", "h@1^-", "i@2^-", "x@3^a"},
		},
		{
			"depth 3: each depth on its own line, linked to its parent",
			agentsOf("a", "b", "a1:a", "b1:b", "a11:a1"),
			want{"a@0^-", "b@0^-", "a1@1^a", "b1@1^b", "a11@2^a1"},
		},
		{
			"every depth is drawn, however deep",
			agentsOf("a", "a1:a", "a2:a1", "a3:a2", "a4:a3"),
			want{"a@0^-", "a1@1^a", "a2@2^a1", "a3@3^a2", "a4@4^a3"},
		},
		{
			"unknown parent hangs off the lead",
			agentsOf("x:gone"),
			want{"x@0^-"},
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
				return tt.agents[nodes[i].Agent].ID
			}
			var got []string
			for i, n := range nodes {
				if n.Rect.W != box.W || n.Rect.H != box.H || (n.Rect.Y-area.Y)%(box.H+rowGap) != 0 {
					t.Fatalf("node %d rect %+v is off the line grid", i, n.Rect)
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

func TestLayoutTreeCentersLines(t *testing.T) {
	nodes := layoutTree(agentsOf("a"), Rect{10, 0, 41, 0}, Rect{W: 21, H: 5}, colGap)
	if len(nodes) != 1 || nodes[0].Rect.X != 20 {
		t.Fatalf("nodes = %+v, want one box at x 20", nodes)
	}
}

// sizes are the terminal sizes the view is checked at: short, narrow, the narrowest and wide.
var sizes = [][2]int{{90, 24}, {60, 20}, {52, 15}, {90, 46}, {200, 50}}

func TestNewLayout(t *testing.T) {
	for _, size := range sizes {
		w, h := size[0], size[1]
		t.Run(fmt.Sprintf("%dx%d", w, h), func(t *testing.T) {
			l := NewLayout(w, h)
			if l.Frame != (Rect{0, 0, w, h}) {
				t.Errorf("frame %+v, want the whole terminal", l.Frame)
			}
			if l.Box.W < 28 || l.Box.W > 30 {
				t.Errorf("box width %d, want 28 to 30", l.Box.W)
			}
			if l.Session.X != 0 || l.Session.Y != 3 || l.Session.W != sessionW || l.Session.Y+l.Session.H != l.Log.Y-1 {
				t.Errorf("session column %+v does not run from below the legend to a row above the log %+v", l.Session, l.Log)
			}
			if want := (Rect{sessionW + 1, 3, w - sessionW - 1, l.Log.Y - 4}); l.View != want {
				t.Errorf("viewport %+v, want %+v: right of the session column, from below the legend to a row above the log", l.View, want)
			}
			if l.Log != (Rect{0, h - 5, w, 4}) || l.Status != h-1 {
				t.Errorf("log %+v, status %d; want the 4 rows above the status line on the last row", l.Log, l.Status)
			}
		})
	}
}

func TestNewLayoutOfATinyTerminal(t *testing.T) {
	for _, size := range [][2]int{{0, 0}, {10, 3}, {30, 8}} {
		l := NewLayout(size[0], size[1])
		if l.View.W < 0 || l.View.H < 0 || l.Session.H < 0 {
			t.Errorf("%v: viewport %+v, session %+v have a negative size", size, l.View, l.Session)
		}
		if l.Box.W < 28 {
			t.Errorf("%v: box width %d, want at least 28", size, l.Box.W)
		}
	}
}

// many returns n agents of the lead and the extra "id:parent" specs.
func many(n int, extra ...string) []session.Agent {
	specs := make([]string, n)
	for i := range specs {
		specs[i] = fmt.Sprintf("agent-%02d", i)
	}
	return agentsOf(append(specs, extra...)...)
}

// TestDiagramDrawsEveryAgent lays out agent trees at every size and checks that every agent gets a box of at least
// 28 columns inside the diagram, the lines they take, and that every box is connected to its parent without a
// connector crossing a box.
func TestDiagramDrawsEveryAgent(t *testing.T) {
	depth3 := agentsOf("a", "b", "a1:a", "b1:b", "a11:a1")
	lines := map[[2]int]map[string]string{ // boxes per line, top down, by size and agent set
		{90, 24}:  {"1": "[1]", "5": "[2 2 1]", "7": "[2 2 2 1]", "12": "[2 2 2 2 2 2]", "depth3": "[2 2 1]"},
		{60, 20}:  {"1": "[1]", "5": "[1 1 1 1 1]", "7": "[1 1 1 1 1 1 1]", "12": "[1 1 1 1 1 1 1 1 1 1 1 1]", "depth3": "[1 1 1 1 1]"},
		{52, 15}:  {"1": "[1]", "5": "[1 1 1 1 1]", "7": "[1 1 1 1 1 1 1]", "12": "[1 1 1 1 1 1 1 1 1 1 1 1]", "depth3": "[1 1 1 1 1]"},
		{90, 46}:  {"1": "[1]", "5": "[2 2 1]", "7": "[2 2 2 1]", "12": "[2 2 2 2 2 2]", "depth3": "[2 2 1]"},
		{200, 50}: {"1": "[1]", "5": "[5]", "7": "[5 2]", "12": "[5 5 2]", "depth3": "[2 2 1]"},
	}
	sets := map[string][]session.Agent{"1": many(1), "5": many(5), "7": many(7), "12": many(12), "depth3": depth3}
	for _, size := range sizes {
		for _, name := range []string{"1", "5", "7", "12", "depth3"} {
			agents := sets[name]
			t.Run(fmt.Sprintf("%s agents at %dx%d", name, size[0], size[1]), func(t *testing.T) {
				l := NewLayout(size[0], size[1])
				d := l.Diagram(agents)
				if d.W < l.View.W || d.H < l.View.H {
					t.Errorf("diagram %dx%d is smaller than the viewport %+v", d.W, d.H, l.View)
				}
				if d.Lead.X < 0 || d.Lead.X+d.Lead.W > d.W || d.Tools.Y < d.Lead.Y+d.Lead.H {
					t.Errorf("lead %+v and tools %+v do not fit the diagram's width %d one below the other", d.Lead, d.Tools, d.W)
				}
				seen := map[int]bool{}
				var counts []int
				lastY := -1
				for _, n := range d.Nodes {
					seen[n.Agent] = true
					if n.Rect.W < 28 {
						t.Errorf("box %+v is narrower than 28", n.Rect)
					}
					if n.Rect.X < 0 || n.Rect.X+n.Rect.W > d.W || n.Rect.Y < d.Tools.Y+d.Tools.H || n.Rect.Y+n.Rect.H > d.H {
						t.Errorf("box %+v leaves the %dx%d diagram or overlaps the tools %+v", n.Rect, d.W, d.H, d.Tools)
					}
					if n.Rect.Y != lastY {
						counts = append(counts, 0)
						lastY = n.Rect.Y
					}
					counts[len(counts)-1]++
				}
				if len(d.Nodes) != len(agents) || len(seen) != len(agents) {
					t.Errorf("%d boxes of %d distinct agents, want one box per agent of %d", len(d.Nodes), len(seen), len(agents))
				}
				if got, want := fmt.Sprint(counts), lines[size][name]; got != want {
					t.Errorf("boxes per line %s, want %s", got, want)
				}
				for i, p := range treePaths(d.Nodes, d.Tools, d.Tree) {
					n := d.Nodes[i]
					from := Point{d.Tools.X + d.Tools.W/2, d.Tools.Y + d.Tools.H}
					if n.Parent >= 0 {
						pr := d.Nodes[n.Parent].Rect
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
						if pt.X < 0 || pt.X >= d.W {
							t.Fatalf("connector of node %d leaves the diagram at %v", i, pt)
						}
						for _, o := range d.Nodes {
							if r := o.Rect; pt.X >= r.X && pt.X < r.X+r.W && pt.Y >= r.Y && pt.Y < r.Y+r.H {
								t.Fatalf("connector of node %d crosses box %+v at %v", i, r, pt)
							}
						}
					}
				}
			})
		}
	}
}

func TestDiagramBoxFitsTheLongestName(t *testing.T) {
	tests := []struct {
		name  string
		w     int
		agent string
		want  int
	}{
		{"short names keep the layout's width", 90, "a", 30},
		{"a long name widens the box", 200, "a-rather-long-agent-name-here", 35},
		{"at most the viewport's width", 90, strings.Repeat("x", 120), 90 - sessionW - 1},
		{"at most the lead's width on a narrow terminal", 52, strings.Repeat("x", 120), leadW},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewLayout(tt.w, 46).Diagram(agentsOf(tt.agent))
			if len(d.Nodes) != 1 || d.Nodes[0].Rect.W != tt.want {
				t.Errorf("nodes %+v, want one box %d wide", d.Nodes, tt.want)
			}
		})
	}
}
