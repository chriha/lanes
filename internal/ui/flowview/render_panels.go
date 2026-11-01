package flowview

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var border = lipgloss.RoundedBorder()

// fit strips escape sequences from s (transcript text is untrusted) and cuts it to w runes, ending in … when cut.
// Runes, not display width: the canvas draws every rune in one cell.
func fit(s string, w int) []rune {
	r := []rune(ansi.Strip(s))
	if w <= 0 {
		return nil
	}
	if len(r) > w {
		r = append(r[:w-1], '…')
	}
	return r
}

// textIn draws s at x, y cut to w cells.
func (c *Canvas) textIn(x, y, w int, s string, role Role) int {
	return c.Text(x, y, string(fit(s, w)), role)
}

// center draws s centered in rect's inside on row y, cut to fit.
func (c *Canvas) center(r Rect, y int, s string, role Role) {
	t := fit(s, r.W-2)
	c.Text(r.X+(r.W-len(t))/2, y, string(t), role)
}

// runeLen is the cells s takes once fit strips its escape sequences.
func runeLen(s string) int { return len([]rune(ansi.Strip(s))) }

// seg is a run of text in one role.
type seg struct {
	s    string
	role Role
}

// segs draws the runs from x, y one after the other, cut to w cells in all, and returns the x after the last.
func (c *Canvas) segs(x, y, w int, runs ...seg) int {
	end := x + w
	for _, r := range runs {
		if x >= end {
			break
		}
		x = c.textIn(x, y, end-x, r.s, r.role)
	}
	return x
}

// centerSegs draws the runs centered in rect's inside on row y, cut to fit.
func (c *Canvas) centerSegs(r Rect, y int, runs ...seg) {
	n := 0
	for _, run := range runs {
		n += runeLen(run.s)
	}
	n = min(n, r.W-2)
	c.segs(r.X+(r.W-n)/2, y, n, runs...)
}

// ctxRow draws a context gauge in w cells from x: the label, a shimmering bar and the percent, or -- when unknown.
func (c *Canvas) ctxRow(x, y, w, ctx, phase int, fill, text Role) {
	pct := "  --"
	if ctx >= 0 {
		pct = fmt.Sprintf("%3d%%", ctx)
	}
	c.Text(x, y, "ctx", RoleDim)
	c.Bar(x+4, y, w-9, float64(max(ctx, 0))/100, phase, fill, RoleDim)
	c.Text(x+w-4, y, pct, text)
}

// spread draws left at the start and right at the end of w cells from x.
func (c *Canvas) spread(x, y, w int, left, right string, lr, rr Role) {
	n := len([]rune(right))
	c.textIn(x, y, w-n-1, left, lr)
	c.Text(x+w-n, y, right, rr)
}

// titled draws a box with title in its top border.
func (c *Canvas) titled(r Rect, title string, role, titleRole Role) {
	c.Box(r, border, role)
	if title != "" {
		c.Text(r.X+2, r.Y, " "+string(fit(title, r.W-6))+" ", titleRole)
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
		for _, fam := range knownFamilies {
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
	c.center(r, r.Y+3, strings.TrimSpace(fmt.Sprintf("%c %s %s", mark, s.leadState(), since(s.Start, s.Now))), role)
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
	aw := s.actorWidth()
	n := r.H - 1
	lines := s.Log[max(len(s.Log)-n, 0):]
	for i, ln := range lines {
		y := r.Y + 1 + i
		x := c.Text(r.X+1, y, ln.At.Format("15:04:05"), RoleDim) + 1
		c.textIn(x, y, aw, ln.Actor, ModelRole(ln.Model))
		c.textIn(x+aw+1, y, r.X+r.W-x-aw-2, ln.Text, RoleText)
	}
}

// maxActorW is the widest the session log's actor column gets.
const maxActorW = 20

// actorWidth is the session log's actor column width: the longest agent or log actor name, up to maxActorW.
func (s State) actorWidth() int {
	w := len("lead")
	for _, a := range s.Snap.Agents {
		w = max(w, runeLen(a.Name))
	}
	for _, ln := range s.Log {
		w = max(w, runeLen(ln.Actor))
	}
	return min(w, maxActorW)
}

// drawStatus draws the status line: an entry per model family of the agents in its color, the totals and whether
// the session is live. Entries that don't fit whole are left out, from the end.
func (s State) drawStatus(c *Canvas, l Layout) {
	var entries [][]seg
	for _, f := range s.familyStatuses() {
		entries = append(entries, []seg{{f.Family + " [" + f.Status + "]", ModelRole(f.Model)}})
	}
	lead := s.Snap.Lead
	for _, kv := range [][2]string{
		{"agents", fmt.Sprintf("%d/%d", s.running(), len(s.Snap.Agents))},
		{"tools", fmtThousands(lead.Calls)},
		{"tokens", fmtThousands(lead.Tokens)},
	} {
		entries = append(entries, []seg{{kv[0] + " ", RoleDim}, {"[" + kv[1] + "]", RoleBold}})
	}
	state := seg{"● live", RoleOK}
	if s.Ended {
		state = seg{"■ ended", RoleBad}
	}
	entries = append(entries, []seg{state})
	x, end := l.Frame.X+1, l.Frame.X+l.Frame.W-1
	for i, e := range entries {
		n := 0
		for _, run := range e {
			n += runeLen(run.s)
		}
		if i > 0 {
			x += 2
		}
		if x+n > end {
			break
		}
		x = c.segs(x, l.Status, n, e...)
	}
}
