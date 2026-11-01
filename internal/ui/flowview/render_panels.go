package flowview

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var border = lipgloss.RoundedBorder()

// textIn draws s at x, y cut to w cells.
func (c *Canvas) textIn(x, y, w int, s string, role Role) int {
	return c.Text(x, y, ansi.Truncate(s, max(w, 0), "…"), role)
}

// center draws s centered in rect's inside on row y, cut to fit.
func (c *Canvas) center(r Rect, y int, s string, role Role) {
	s = ansi.Truncate(s, max(r.W-2, 0), "…")
	c.Text(r.X+(r.W-ansi.StringWidth(s))/2, y, s, role)
}

// spread draws left at the start and right at the end of w cells from x.
func (c *Canvas) spread(x, y, w int, left, right string, lr, rr Role) {
	c.textIn(x, y, w-ansi.StringWidth(right)-1, left, lr)
	c.Text(x+w-ansi.StringWidth(right), y, right, rr)
}

// titled draws a box with title in its top border.
func (c *Canvas) titled(r Rect, title string, role, titleRole Role) {
	c.Box(r, border, role)
	if title != "" {
		c.textIn(r.X+2, r.Y, r.W-4, " "+title+" ", titleRole)
	}
}

func (s State) drawHeader(c *Canvas, l Layout) {
	f := l.Frame
	c.textIn(f.X+1, f.Y, f.W-8, "AGENT VIEW · "+s.Title, RoleBold)
	state, role := "live", RoleOK
	if s.Ended {
		state, role = "ended", RoleBad
	}
	c.Text(f.X+f.W-1-len(state), f.Y, state, role)
	c.HLine(f.X, f.X+f.W-1, f.Y+1, '═', RoleDim)
	if l.Mode == ModeFull {
		x := f.X + (f.W-28)/2
		for _, fam := range []string{"opus", "sonnet", "haiku"} {
			c.Set(x, f.Y+2, '■', ModelRole(fam))
			x = c.Text(x+2, f.Y+2, fam, RoleDim) + 3
		}
	}
}

func (s State) drawSession(c *Canvas, r Rect) {
	c.titled(r, "session", RoleDim, RoleBold)
	x, w, y := r.X+2, r.W-4, r.Y+1
	for i, h := range hookStates {
		mark, role := '◇', RoleText
		if h == s.Hook && !s.Ended {
			mark, role = '◆', RoleSelected
			c.HLine(r.X+1, r.X+r.W-2, y+i, ' ', role)
		}
		c.Set(x, y+i, mark, role)
		c.Text(x+2, y+i, h, role)
	}
	c.Text(x, y+4, "last:", RoleDim)
	lines := strings.Split(ansi.Wrap(strings.Join(strings.Fields(s.Snap.Lead.LastText), " "), w, ""), "\n")
	if len(lines) > 3 {
		lines[2] += "…"
	}
	for i := 0; i < min(len(lines), 3); i++ {
		c.textIn(x, y+5+i, w, lines[i], RoleText)
	}
	lead := s.Snap.Lead
	for i, kv := range [][2]string{{"turns", fmt.Sprint(lead.Turns)}, {"tools", fmt.Sprint(lead.Calls)}, {"tokens", fmtCount(lead.Tokens)}} {
		c.spread(x, y+8+i, w, kv[0], kv[1], RoleDim, RoleBold)
	}
}

func (s State) drawLead(c *Canvas, r Rect) {
	lead := s.Snap.Lead
	role := ModelRole(lead.Model)
	c.Box(r, border, role)
	c.center(r, r.Y+1, strings.TrimSpace(shortModel(lead.Model)+" · lead"), role)
	if lead.Tool != "" {
		c.center(r, r.Y+2, strings.TrimSpace(lead.Tool+" "+lead.ToolArg), RoleText)
	} else {
		c.center(r, r.Y+2, "idle", RoleDim)
	}
	mark := '◇'
	switch {
	case s.Ended:
		mark = '■'
	case s.Hook == "working":
		mark = s.spin()
	}
	hook := s.Hook
	if s.Ended {
		hook = "ended"
	}
	c.center(r, r.Y+3, fmt.Sprintf("%c %s %s", mark, hook, fmtDur(s.Now.Sub(s.Start))), role)
}

func (s State) drawTools(c *Canvas, r Rect) {
	role := ModelRole(s.Snap.Lead.Model)
	c.Box(r, border, RoleDim)
	x, w := r.X+2, r.W-4
	c.spread(x, r.Y+1, w, "tools", fmt.Sprintf("calls %d", s.Snap.Lead.Calls), RoleBold, RoleBold)
	for i, t := range s.topTools(3) {
		y := r.Y + 2 + i
		c.textIn(x, y, 8, t.Name, RoleText)
		c.Bar(x+9, y, w-14, t.Share, s.Phase, role, RoleDim)
		c.Text(x+w-4, y, fmt.Sprintf("%3.0f%%", t.Share*100), RoleText)
	}
}

func (s State) drawLog(c *Canvas, r Rect) {
	c.HLine(r.X, r.X+r.W-1, r.Y, '─', RoleDim)
	c.Text(r.X+2, r.Y, "┤ session log ├", RoleDim)
	n := r.H - 1
	lines := s.Log[max(len(s.Log)-n, 0):]
	for i, ln := range lines {
		y := r.Y + 1 + i
		x := c.Text(r.X+1, y, ln.At.Format("15:04:05"), RoleDim) + 1
		c.textIn(x, y, 12, ln.Actor, ModelRole(ln.Model))
		c.textIn(x+13, y, r.X+r.W-x-14, ln.Text, RoleText)
	}
}

func (s State) drawStatus(c *Canvas, l Layout) {
	hook := s.Hook
	if s.Ended {
		hook = "ended"
	}
	line := fmt.Sprintf("lead [%s]   agents [%d/%d running]", hook, s.running(), len(s.Snap.Agents))
	if l.Mode == ModeCompact {
		lead := s.Snap.Lead
		line = fmt.Sprintf("lead [%s]  agents [%d/%d]  turns %d  tools %d  tokens %s",
			hook, s.running(), len(s.Snap.Agents), lead.Turns, lead.Calls, fmtCount(lead.Tokens))
	}
	c.textIn(l.Frame.X+1, l.Status, l.Frame.W-2, line, RoleText)
}
