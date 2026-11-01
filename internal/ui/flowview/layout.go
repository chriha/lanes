package flowview

import (
	"sort"

	"github.com/chriha/lanes/internal/session"
)

// Mode is how much of the view fits the terminal.
type Mode int

const (
	ModeTooSmall Mode = iota // only a resize hint
	ModeCompact              // no session column and tool layer, smaller agent boxes
	ModeFull                 // the whole diagram
)

const (
	fullW, fullH     = 90, 46
	minW, minH       = 60, 24
	rowGap           = 3      // rows between a parent and its children: stem, bus, arrow
	colGap           = 2      // columns between compact agent boxes
	wideGap          = 3      // columns between full agent boxes: a connector runs down the middle one
	boxMinW, boxMaxW = 28, 30 // full agent box width
	sessionW         = 22     // the full layout's session column
	leadW            = 36
	maxLines         = 2 // lines one spawn depth may wrap into
)

// Layout holds the panels' rectangles for one terminal size. Rects that the mode leaves out are zero.
type Layout struct {
	Mode    Mode
	Frame   Rect // the drawn area: the whole terminal in the full layout, else centered in it
	Session Rect
	Lead    Rect
	Tools   Rect
	Tree    Rect // where the agent rows go
	Box     Rect // the size (W, H) of one agent box
	Gap     int  // columns between agent boxes side by side
	Log     Rect // the log's title rule is its first row
	Status  int  // row of the status line
}

// NewLayout lays the view out for a w×h terminal: full from 90×46, compact from 60×24, else too small. The full
// layout fills the terminal and fits as many agent boxes of 28 to 30 columns side by side as the width allows.
func NewLayout(w, h int) Layout {
	if w < minW || h < minH {
		return Layout{Mode: ModeTooSmall, Frame: Rect{0, 0, w, h}}
	}
	var l Layout
	if w >= fullW && h >= fullH {
		l = Layout{Mode: ModeFull, Frame: Rect{0, 0, w, h}, Status: h - 1, Gap: wideGap}
		lx := sessionW + (w-sessionW-leadW)/2 + 1 // centered right of the session column
		l.Session = Rect{0, 3, sessionW, 13}
		l.Lead = Rect{lx, 3, leadW, 5}
		l.Tools = Rect{lx, 10, leadW, 6}
		slots := (w + wideGap) / (boxMinW + wideGap)
		l.Box = Rect{W: min(boxMaxW, (w+wideGap)/slots-wideGap), H: 5}
		l.Log = Rect{0, h - 5, w, 4}
	} else {
		fw, fh := min(w, fullW), min(h, fullH)
		x, y := (w-fw)/2, (h-fh)/2
		l = Layout{Mode: ModeCompact, Frame: Rect{x, y, fw, fh}, Status: y + fh - 1, Gap: colGap}
		l.Lead = Rect{x + (fw-leadW)/2, y + 2, leadW, 5}
		l.Box = Rect{W: 16, H: 4}
		l.Log = Rect{x, l.Status - 3, fw, 3}
	}
	above := l.Lead
	if l.Mode == ModeFull {
		above = l.Tools
	}
	top := above.Y + above.H + rowGap
	l.Tree = Rect{l.Frame.X, top, l.Frame.W, l.Log.Y - 1 - top}
	return l
}

// Node is one box of the agent tree.
type Node struct {
	Rect   Rect
	Agent  int // index into the agents, -1 for a "+n more" box
	More   int // agents folded into a "+n more" box
	Parent int // index of the parent node, -1 for the lead
}

// layoutTree places the agents in area: one row per spawn depth, gap columns apart, each line centered. A depth
// with more agents than fit one line wraps into a second line, as long as a line stays free for the next depth.
// A depth with more agents than its lines hold, and every agent deeper than the last depth that fits, folds into
// a "+n more" box at the end of the depth's last line, which hangs off the parent of its first agent. An agent
// whose parent is unknown hangs off the lead; one whose parent was folded hangs off the "+n more" box.
func layoutTree(agents []session.Agent, area, box Rect, gap int) []Node {
	lines := (area.H + rowGap) / (box.H + rowGap)
	slots := (area.W + gap) / (box.W + gap)
	if lines < 1 || slots < 1 || len(agents) == 0 {
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
	// span[r] is the number of lines depth r+1 takes; depths past len(span) fold.
	var span []int
	for used := 0; len(span) < len(byDepth) && used < lines; {
		free := lines - used
		if len(span) < len(byDepth)-1 && free > 1 {
			free-- // keep a line for the next depth
		}
		need := (len(byDepth[len(span)]) + slots - 1) / slots
		k := min(need, maxLines, free)
		span = append(span, k)
		used += k
	}
	rows := len(span)
	deeper, firstDeep := 0, -1
	for i := range agents {
		if depth[i] > rows {
			if deeper++; firstDeep < 0 {
				firstDeep = i
			}
		}
	}
	var nodes []Node
	nodeOf := make([]int, len(agents)) // node of each placed or folded agent
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
	for r, row := range byDepth[:rows] {
		sort.SliceStable(row, func(a, b int) bool { return centerX(parentNode(row[a])) < centerX(parentNode(row[b])) })
		folded := 0
		if r == rows-1 {
			folded = deeper
		}
		capacity := span[r] * slots
		shown := len(row)
		if shown > capacity || folded > 0 && shown >= capacity {
			shown = capacity - 1
		}
		folded += len(row) - shown
		k := shown
		if folded > 0 {
			k++
		}
		for start := 0; start < k; start += slots {
			n := min(slots, k-start)
			x := area.X + (area.W-n*box.W-(n-1)*gap)/2
			y := area.Y + line*(box.H+rowGap)
			line++
			for j := start; j < start+n; j++ {
				rect := Rect{x + (j-start)*(box.W+gap), y, box.W, box.H}
				if j < shown {
					nodeOf[row[j]] = len(nodes)
					nodes = append(nodes, Node{Rect: rect, Agent: row[j], Parent: parentNode(row[j])})
					continue
				}
				parent := -1
				switch {
				case shown < len(row):
					parent = parentNode(row[shown])
				case firstDeep >= 0: // hang off the parent of the deep agent's ancestor in this row
					a := firstDeep
					for depth[a] > rows {
						a = byID[agents[a].Parent]
					}
					parent = parentNode(a)
				}
				for _, i := range row[shown:] {
					nodeOf[i] = len(nodes)
				}
				nodes = append(nodes, Node{Rect: rect, Agent: -1, More: folded, Parent: parent})
			}
		}
	}
	return nodes
}
