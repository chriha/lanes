package flowview

import (
	"sort"

	"github.com/chriha/lanes/internal/session"
)

const (
	rowGap           = 3      // rows between a parent and its children: stem, bus, arrow
	colGap           = 2      // columns between boxes in the layout tests
	wideGap          = 3      // columns between agent boxes: a connector runs down the middle one
	boxMinW, boxMaxW = 28, 30 // agent box width
	sessionW         = 22     // the session column
	leadW            = 36
	boxH             = 7 // agent box height: border, model, ctx, usage, task, live text, border
	logLines         = 4 // the lines the session log shows
	procsW           = 50
	procsRows        = 5             // the processes the processes box lists
	procsH           = procsRows + 2 // the processes box height: border, the processes, border
)

// Layout holds the pinned panels' rectangles for one terminal size and the viewport the diagram scrolls in. There
// is one layout at every size: what does not fit the viewport is reached by scrolling.
type Layout struct {
	Frame   Rect // the whole terminal
	Session Rect
	View    Rect // the viewport the diagram scrolls in: right of the session column, between the legend and the log
	Box     Rect // the size (W, H) of one agent box
	Gap     int  // columns between agent boxes side by side
	Log     Rect // the log box: a border around logLines lines
	Status  int  // row of the status line
}

// NewLayout lays the view out for a w×h terminal: the header and legend on top, the session column down the left
// side, the log and the status line at the bottom, and the viewport right of the session column. As many agent
// boxes of 28 to 30 columns go side by side as the viewport's width allows, at least one: boxes never shrink below
// 28, a narrower viewport scrolls sideways.
func NewLayout(w, h int) Layout {
	l := Layout{Frame: Rect{0, 0, w, h}, Status: h - 1, Gap: wideGap}
	l.Log = Rect{0, h - 3 - logLines, w, logLines + 2}
	l.Session = Rect{0, 3, sessionW, max(l.Log.Y-4, 0)} // down to a row above the log
	l.View = Rect{sessionW + 1, 3, max(w-sessionW-1, 0), l.Session.H}
	slots := max((l.View.W+wideGap)/(boxMinW+wideGap), 1)
	l.Box = Rect{W: max(min(boxMaxW, (l.View.W+wideGap)/slots-wideGap), boxMinW), H: boxH}
	return l
}

// Diagram is the scrolling part of the view laid out on a canvas of its own, in its own coordinates: the lead with
// the tools box right of it, and the agent tree below the lead. It is at least the viewport's size.
type Diagram struct {
	W, H  int
	Lead  Rect
	Tools Rect // left of the lead on its top row, joined to nothing
	Procs Rect // right of the lead on the top row, joined to nothing; empty without processes
	Tree  Rect // where the agent rows go
	Box   Rect // the size (W, H) of one agent box
	Nodes []Node
}

// Diagram lays out the scrolling part for the agents: the lead centered at the top with the tools box wideGap
// columns left of it and, with procs, the processes box wideGap columns right of it; every agent below the lead. A
// viewport too narrow for the tools box left of the centered lead shifts the lead right until the tools box sits at 0.
// It is as wide as the viewport, the lead or an agent box, whichever is widest, and wider when the side boxes end
// past that; as high as the viewport or its agents, whichever is higher. The agents are laid out in the width
// without the side boxes, so they add no columns to a line. The processes box is higher than the tools box: the
// agents go a row lower with it, so the connectors' bus runs below it.
func (l Layout) Diagram(agents []session.Agent, procs bool) Diagram {
	box := l.Box
	box.W = l.boxWidth(agents)
	w := max(l.View.W, leadW, box.W) // the tree's width: the lead is centered over it
	d := Diagram{Box: box, Lead: Rect{(w - leadW + 1) / 2, 0, leadW, 5}}
	d.Lead.X = max(d.Lead.X, leadW+wideGap) // no room left of the centered lead: shift it right, the diagram scrolls
	d.Tools = Rect{d.Lead.X - wideGap - leadW, d.Lead.Y, leadW, 6}
	d.W = max(w, d.Lead.X+leadW)
	d.Tree = Rect{0, d.Lead.Y + d.Lead.H + rowGap, w, 0}
	bottom := max(d.Lead.Y+d.Lead.H, d.Tools.Y+d.Tools.H)
	if procs {
		d.Procs = Rect{d.Lead.X + d.Lead.W + wideGap, d.Lead.Y, procsW, procsH}
		d.W = max(d.W, d.Procs.X+d.Procs.W)
		d.Tree.Y = max(d.Tree.Y, d.Procs.Y+d.Procs.H+rowGap-1) // the bus runs two rows above the agents
		bottom = max(bottom, d.Procs.Y+d.Procs.H)
	}
	d.Nodes = layoutTree(agents, d.Tree, box, l.Gap)
	for _, n := range d.Nodes {
		bottom = max(bottom, n.Rect.Y+n.Rect.H)
	}
	d.Tree.H = max(bottom-d.Tree.Y, 0)
	d.H = max(bottom, l.View.H)
	return d
}

// boxWidth is the agent box width: the layout's, widened so the longest agent name fits the box title, at most the
// viewport's or the lead's width, whichever is wider.
func (l Layout) boxWidth(agents []session.Agent) int {
	w := l.Box.W
	for _, a := range agents {
		w = max(w, runeLen(a.Name)+6) // corner, rule and a space on each side
	}
	return min(w, max(l.View.W, leadW))
}

// Node is one box of the agent tree.
type Node struct {
	Rect   Rect
	Agent  int // index into the agents
	Parent int // index of the parent node, -1 for the lead
}

// layoutTree places every agent from the top of area down: one row per spawn depth, each row wrapped into as many
// lines of boxes gap columns apart as the area's width needs, each line centered. An agent whose parent is unknown
// hangs off the lead.
func layoutTree(agents []session.Agent, area, box Rect, gap int) []Node {
	slots := max((area.W+gap)/(box.W+gap), 1)
	if len(agents) == 0 {
		return nil
	}
	byID := map[string]int{}
	for i, a := range agents {
		byID[a.ID] = i
	}
	depth := make([]int, len(agents))
	var depthOf func(i int) int
	depthOf = func(i int) int {
		switch depth[i] {
		case -1: // on the current parent chain: a cycle, cut here
			return 0
		case 0:
			depth[i] = -1
			d := 1
			if p, ok := byID[agents[i].Parent]; ok {
				d = depthOf(p) + 1
			}
			depth[i] = d
		}
		return depth[i]
	}
	var byDepth [][]int
	for i := range agents {
		d := depthOf(i)
		for len(byDepth) < d {
			byDepth = append(byDepth, nil)
		}
		byDepth[d-1] = append(byDepth[d-1], i)
	}
	nodes := make([]Node, 0, len(agents))
	nodeOf := make([]int, len(agents))
	parentNode := func(i int) int {
		if p, ok := byID[agents[i].Parent]; ok && depth[p] < depth[i] {
			return nodeOf[p]
		}
		return -1
	}
	centerX := func(n int) int {
		if n < 0 {
			return area.X + area.W/2
		}
		return nodes[n].Rect.X + nodes[n].Rect.W/2
	}
	line := 0
	for _, row := range byDepth {
		sort.SliceStable(row, func(a, b int) bool { return centerX(parentNode(row[a])) < centerX(parentNode(row[b])) })
		for start := 0; start < len(row); start += slots {
			n := min(slots, len(row)-start)
			x := area.X + (area.W-n*box.W-(n-1)*gap)/2
			y := area.Y + line*(box.H+rowGap)
			line++
			for j := start; j < start+n; j++ {
				nodeOf[row[j]] = len(nodes)
				nodes = append(nodes, Node{Rect: Rect{x + (j-start)*(box.W+gap), y, box.W, box.H}, Agent: row[j], Parent: parentNode(row[j])})
			}
		}
	}
	return nodes
}
