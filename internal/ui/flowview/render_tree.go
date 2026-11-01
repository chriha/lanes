package flowview

import (
	"fmt"
	"strings"

	"github.com/chriha/lanes/internal/session"
)

// treePaths returns each node's connector from its parent; the lead's connectors start below root. Connectors
// stay inside area.
func treePaths(nodes []Node, root, area Rect) [][]Point {
	below := func(r Rect) Point { return Point{r.X + r.W/2, r.Y + r.H} }
	paths := make([][]Point, len(nodes))
	boxes := make([]Rect, len(nodes))
	for i, n := range nodes {
		boxes[i] = n.Rect
	}
	for i, n := range nodes {
		from := below(root)
		if n.Parent >= 0 {
			from = below(nodes[n.Parent].Rect)
		}
		paths[i] = route(from, Point{n.Rect.X + n.Rect.W/2, n.Rect.Y - 1}, boxes, area)
	}
	return paths
}

// toolPath is the full layout's connector from the lead to the tool layer, nil in the other layouts.
func toolPath(l Layout) []Point {
	if l.Mode != ModeFull {
		return nil
	}
	return elbow(Point{l.Lead.X + l.Lead.W/2, l.Lead.Y + l.Lead.H}, Point{l.Tools.X + l.Tools.W/2, l.Tools.Y - 1})
}

// drawTree draws the agent boxes and their connectors and returns the boxes and each one's connector.
func (s State) drawTree(c *Canvas, l Layout) ([]Node, [][]Point) {
	root := l.Lead
	if l.Mode == ModeFull {
		root = l.Tools
		c.drawPaths([][]Point{toolPath(l)}, RoleDim)
	}
	box := l.Box
	box.W = s.boxWidth(l)
	nodes := layoutTree(s.Snap.Agents, l.Tree, box, l.Gap)
	paths := treePaths(nodes, root, l.Tree)
	c.drawPaths(paths, RoleDim)
	for _, n := range nodes {
		if n.Agent < 0 {
			c.Box(n.Rect, border, RoleDim)
			c.center(n.Rect, n.Rect.Y+n.Rect.H/2, fmt.Sprintf("+%d more", n.More), RoleDim)
			continue
		}
		s.drawAgent(c, n.Rect, s.Snap.Agents[n.Agent], l.Mode == ModeFull)
	}
	return nodes, paths
}

// boxWidth is the agent box width: the layout's, widened so the longest agent name fits the box title, at most
// the tree area's width.
func (s State) boxWidth(l Layout) int {
	w := l.Box.W
	for _, a := range s.Snap.Agents {
		w = max(w, runeLen(a.Name)+6) // corner, rule and a space on each side
	}
	return min(w, l.Tree.W)
}

// drawAgent draws an agent box: the name in the top border; the model and effort meter, the context bar and the
// task (full only) inside; the status and, while running, the current activity in the bottom border. A finished
// agent's box is dim and still.
func (s State) drawAgent(c *Canvas, r Rect, a session.Agent, full bool) {
	running := a.Status == session.AgentRunning && !s.Ended // an ended session's agents are stopped
	role, title, text, phase := ModelRole(a.Model), RoleBold, RoleText, s.Phase
	if !running {
		role, title, text, phase = RoleDim, RoleDim, RoleDim, 0
	}
	c.titled(r, a.Name, role, title)
	model := shortModel(a.Model)
	if !full {
		model, _, _ = strings.Cut(model, " ")
	}
	row := []seg{{model, role}}
	if m := effortMeter(a.Effort); m != "" {
		row = append(row, seg{" " + m, role})
		if full {
			row = append(row, seg{" " + strings.ToLower(a.Effort), RoleDim})
		}
	}
	c.centerSegs(r, r.Y+1, row...)
	c.ctxRow(r.X+2, r.Y+2, r.W-4, a.Ctx, phase, role, text)
	if full {
		c.center(r, r.Y+3, a.Task, text)
	}
	y := r.Y + r.H - 1
	x := c.Text(r.X+2, y, " ", role)
	x = c.segs(x, y, r.W-6, s.agentStatus(a, role)...)
	c.Text(x, y, " ", role)
}

// agentStatus is an agent's status line: spinner, elapsed time and current activity while running; stopped once the
// session ended; done or failed with the duration otherwise.
func (s State) agentStatus(a session.Agent, role Role) []seg {
	join := func(parts ...string) string { return strings.Join(strings.Fields(strings.Join(parts, " ")), " ") }
	switch a.Status {
	case session.AgentRunning:
		if s.Ended {
			return []seg{{join("■ stopped", since(a.Start, s.Now)), RoleDim}}
		}
		mark := string(s.spin())
		if a.Tool == "" {
			return []seg{{join(mark, "running", since(a.Start, s.Now)), role}}
		}
		return []seg{{join(mark, since(a.Start, s.Now)), role}, {" ▸ ", RoleDim}, {join(a.Tool, a.ToolArg), RoleText}}
	case session.AgentDone:
		return []seg{{join("✓ done", since(a.Start, a.End)), RoleOK}}
	}
	return []seg{{join("✗ failed", since(a.Start, a.End)), RoleBad}}
}
