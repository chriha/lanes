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
	fullW, fullH = 90, 46
	minW, minH   = 60, 24
	rowGap       = 3 // rows between a parent and its children: stem, bus, arrow
	colGap       = 2
)

// Layout holds the panels' rectangles for one terminal size. Rects that the mode leaves out are zero.
type Layout struct {
	Mode    Mode
	Frame   Rect // the drawn area, centered in the terminal
	Session Rect
	Lead    Rect
	Tools   Rect
	Tree    Rect // where the agent rows go
	Box     Rect // the size (W, H) of one agent box
	Log     Rect // the log's title rule is its first row
	Status  int  // row of the status line
}

// NewLayout lays the view out for a w×h terminal: full from 90×46, compact from 60×24, else too small.
func NewLayout(w, h int) Layout {
	if w < minW || h < minH {
		return Layout{Mode: ModeTooSmall, Frame: Rect{0, 0, w, h}}
	}
	fw, fh := min(w, fullW), min(h, fullH)
	x, y := (w-fw)/2, (h-fh)/2
	l := Layout{Frame: Rect{x, y, fw, fh}, Status: y + fh - 1}
	if w >= fullW && h >= fullH {
		l.Mode = ModeFull
		l.Session = Rect{x, y + 3, 22, 13}
		l.Lead = Rect{x + 39, y + 3, 36, 5}
		l.Tools = Rect{x + 39, y + 10, 36, 6}
		l.Box = Rect{W: 21, H: 5}
		l.Log = Rect{x, y + 41, fw, 4}
	} else {
		l.Mode = ModeCompact
		l.Lead = Rect{x + (fw-36)/2, y + 2, 36, 5}
		l.Box = Rect{W: 16, H: 4}
		l.Log = Rect{x, l.Status - 3, fw, 3}
	}
	above := l.Lead
	if l.Mode == ModeFull {
		above = l.Tools
	}
	top := above.Y + above.H + rowGap
	l.Tree = Rect{x, top, fw, l.Log.Y - 1 - top}
	return l
}

// Node is one box of the agent tree.
type Node struct {
	Rect   Rect
	Agent  int // index into the agents, -1 for a "+n more" box
	More   int // agents folded into a "+n more" box
	Parent int // index of the parent node, -1 for the lead
}

// layoutTree places the agents in area: one row per spawn depth, each row centered. A row with more agents than
// fit, and every agent deeper than the last row that fits, folds into a "+n more" box at the row's end, which hangs
// off the parent of its first agent. An agent whose parent is unknown hangs off the lead; one whose parent was
// folded hangs off the "+n more" box.
func layoutTree(agents []session.Agent, area, box Rect) []Node {
	rows := (area.H + rowGap) / (box.H + rowGap)
	slots := (area.W + colGap) / (box.W + colGap)
	if rows < 1 || slots < 1 || len(agents) == 0 {
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
	members := make([][]int, rows)
	deeper, firstDeep := 0, -1
	for i := range agents {
		if d := depthOf(i); d <= rows {
			members[d-1] = append(members[d-1], i)
		} else if deeper++; firstDeep < 0 {
			firstDeep = i
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
	for r, row := range members {
		sort.SliceStable(row, func(a, b int) bool { return centerX(parentNode(row[a])) < centerX(parentNode(row[b])) })
		folded := 0
		if r == rows-1 {
			folded = deeper
		}
		shown := len(row)
		if shown > slots || folded > 0 && shown >= slots {
			shown = slots - 1
		}
		folded += len(row) - shown
		k := shown
		if folded > 0 {
			k++
		}
		if k == 0 {
			continue
		}
		x := area.X + (area.W-k*box.W-(k-1)*colGap)/2
		y := area.Y + r*(box.H+rowGap)
		for j := 0; j < k; j++ {
			rect := Rect{x + j*(box.W+colGap), y, box.W, box.H}
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
	return nodes
}
