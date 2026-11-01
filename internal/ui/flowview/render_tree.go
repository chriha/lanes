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

// drawTree draws the agent boxes and their connectors and returns the boxes and each one's connector.
func (s State) drawTree(c *Canvas, l Layout) ([]Node, [][]Point) {
	root := l.Lead
	if l.Mode == ModeFull {
		root = l.Tools
		c.drawPaths([][]Point{elbow(Point{l.Lead.X + l.Lead.W/2, l.Lead.Y + l.Lead.H}, Point{l.Tools.X + l.Tools.W/2, l.Tools.Y - 1})}, RoleDim)
	}
	nodes := layoutTree(s.Snap.Agents, l.Tree, l.Box, l.Gap)
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

// drawAgent draws an agent box: name in the border, model and ctx %, the task (full only) and the status.
func (s State) drawAgent(c *Canvas, r Rect, a session.Agent, full bool) {
	role, text := ModelRole(a.Model), RoleText
	if a.Status != session.AgentRunning {
		role, text = RoleDim, RoleDim
	}
	c.titled(r, a.Name, role, role)
	model := shortModel(a.Model)
	if !full {
		model, _, _ = strings.Cut(model, " ")
	}
	if a.Ctx >= 0 {
		model = strings.TrimSpace(fmt.Sprintf("%s ctx %d%%", model, a.Ctx))
	}
	c.center(r, r.Y+1, model, text)
	if full {
		c.center(r, r.Y+2, a.Task, text)
	}
	var status string
	srole := RoleText
	switch a.Status {
	case session.AgentRunning:
		mark := s.spin()
		if s.Ended { // frozen: no spinner
			mark = '■'
		}
		status = fmt.Sprintf("%c %s %s", mark, since(a.Start, s.Now), a.Tool)
	case session.AgentDone:
		status, srole = "✓ "+since(a.Start, a.End), RoleOK
	default:
		status, srole = "✗ "+since(a.Start, a.End), RoleBad
	}
	status = strings.Join(strings.Fields(status), " ")
	c.center(r, r.Y+r.H-2, status, srole)
}
