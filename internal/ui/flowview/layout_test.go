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
	area := Rect{0, 0, 90, 21} // the full layout's tree area: 4 boxes of 21 per row, 3 rows
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
			"seven agents overflow into +n more",
			agentsOf("a", "b", "c", "d", "e", "f", "g"),
			want{"a@0^-", "b@0^-", "c@0^-", "+4@0^-"},
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
			agentsOf("a", "b", "c", "d", "e", "e1:e"),
			want{"a@0^-", "b@0^-", "c@0^-", "+2@0^-", "e1@1^+2"},
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
		{120, 60, ModeFull, Rect{15, 7, 90, 46}},
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
			if l.Tree.H < l.Box.H {
				t.Errorf("tree area %+v holds no row of %d", l.Tree, l.Box.H)
			}
			if l.Tree.Y+l.Tree.H >= l.Log.Y || l.Log.Y+l.Log.H > l.Status {
				t.Errorf("tree %+v, log %+v and status %d overlap", l.Tree, l.Log, l.Status)
			}
		})
	}
}
