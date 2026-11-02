package flowview

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/session"
)

// border frames the panels and agent boxes; leadBorder sets the lead apart; logBorder closes the session log.
var border, leadBorder, logBorder = lipgloss.ThickBorder(), lipgloss.DoubleBorder(), lipgloss.RoundedBorder()

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

// segsLen is the cells the runs take.
func segsLen(runs []seg) int {
	n := 0
	for _, r := range runs {
		n += runeLen(r.s)
	}
	return n
}

// centerSegs draws the runs centered in rect's inside on row y, cut to fit.
func (c *Canvas) centerSegs(r Rect, y int, runs ...seg) {
	n := min(segsLen(runs), r.W-2)
	c.segs(r.X+(r.W-n)/2, y, n, runs...)
}

// ctxPct is a context use as a whole percent, -- when unknown.
func ctxPct(ctx int) string {
	if ctx < 0 {
		return "--"
	}
	return fmt.Sprintf("%d%%", ctx)
}

// ctxFill is a context use as the fraction a gauge is filled to, empty when unknown.
func ctxFill(ctx int) float64 { return float64(max(ctx, 0)) / 100 }

// ctxRow draws a context gauge in w cells from x: the label, the gauge and the percent.
func (c *Canvas) ctxRow(x, y, w, ctx int, fill, text Role) {
	c.Text(x, y, "ctx", RoleDim)
	c.Gauge(x+4, y, w-9, ctxFill(ctx), fill, RoleTrack)
	c.Text(x+w-4, y, fmt.Sprintf("%4s", ctxPct(ctx)), text)
}

// spread draws left at the start and right at the end of w cells from x.
func (c *Canvas) spread(x, y, w int, left, right string, lr, rr Role) {
	n := len([]rune(right))
	c.textIn(x, y, w-n-1, left, lr)
	c.Text(x+w-n, y, right, rr)
}

// titled draws a box with title in its top border.
func (c *Canvas) titled(r Rect, title string, role, titleRole Role) {
	c.titledIn(r, border, title, role, titleRole)
}

// titledIn draws a box in border b with title in its top border.
func (c *Canvas) titledIn(r Rect, b lipgloss.Border, title string, role, titleRole Role) {
	c.Box(r, b, role)
	if title != "" {
		c.Text(r.X+2, r.Y, " "+string(fit(title, r.W-6))+" ", titleRole)
	}
}

// drawHeader draws the header, AGENT VIEW and the item's title (the session's name when unknown), its rule and the
// legend of the model families under it.
func (s State) drawHeader(c *Canvas, l Layout) {
	f := l.Frame
	title := s.Item
	if title == "" {
		title = s.Title
	}
	c.textIn(f.X+1, f.Y, f.W-2, "AGENT VIEW · "+title, RoleAccent)
	c.HLine(f.X, f.X+f.W-1, f.Y+1, '═', RoleAccent)
	x := f.X + (f.W-28)/2
	for _, fam := range knownFamilies {
		c.Set(x, f.Y+2, '■', ModelRole(fam))
		x = c.Text(x+2, f.Y+2, fam, RoleDim) + 3
	}
}

// drawPosition shows on the row above the log, right-aligned, which lines and columns of the diagram d the viewport
// shows from x, y: the lines when the diagram is higher than the viewport, the columns when it is wider. A focused
// diagram is named there in the accent, with its position; an unfocused one that fits shows nothing.
func drawPosition(c *Canvas, l Layout, d Diagram, x, y int, focused bool) {
	v := l.View
	var parts []string
	role := RoleDim
	if focused {
		parts, role = append(parts, "diagram"), RoleAccent
	}
	if d.H > v.H {
		parts = append(parts, fmt.Sprintf("lines %d-%d/%d", y+1, y+v.H, d.H))
	}
	if d.W > v.W {
		parts = append(parts, fmt.Sprintf("cols %d-%d/%d", x+1, x+v.W, d.W))
	}
	if len(parts) == 0 {
		return
	}
	s := strings.Join(parts, "  ")
	c.Text(l.Frame.X+l.Frame.W-1-len(s), l.Log.Y-1, s, role)
}

// panelRoles are the border and title roles of a pinned panel: the accent when it has the focus.
func panelRoles(focused bool) (Role, Role) {
	if focused {
		return RoleAccent, RoleAccent
	}
	return RoleDim, RoleBold
}

// drawSession draws the session column as the lead's dashboard, its rows from row off on: those past the column's
// bottom are left out. A focused column has its border and title in the accent.
func (s State) drawSession(c *Canvas, r Rect, off int, focused bool) {
	role, title := panelRoles(focused)
	c.titled(r, "session", role, title)
	rows := s.sessionRows(c, r)
	rows = rows[min(max(off, 0), len(rows)):]
	for i, draw := range rows[:max(min(len(rows), r.H-2), 0)] {
		if draw != nil {
			draw(r.Y + 1 + i)
		}
	}
}

// sessionRows are the session column's rows, each drawing itself on the row it is given (nil for a blank one):
// the session's name and age, the lead's context and effort; the tool calls per minute as a sparkline; the hook
// states on a timeline; the counters; the files touched last; the lead's last text; and the advisor when there
// is one.
func (s State) sessionRows(c *Canvas, r Rect) []func(y int) {
	x, w := r.X+2, r.W-4
	lead := s.Snap.Lead
	role := ModelRole(lead.Model)
	label := func(name, value string, vr Role) func(y int) {
		return func(y int) { c.spread(x, y, w, name, value, RoleDim, vr) }
	}
	rows := []func(y int){
		label(s.Title, since(s.Start, s.Now), RoleText),
		label("ctx", ctxPct(lead.Ctx), RoleBold),
		func(y int) { c.Gauge(x, y, w, ctxFill(lead.Ctx), role, RoleTrack) },
	}
	if m := effortMeter(lead.Effort); m != "" {
		rows = append(rows, func(y int) {
			c.segs(x, y, w, seg{"effort ", RoleDim}, seg{m, role}, seg{" " + strings.ToLower(lead.Effort), RoleText})
		})
	}
	counts := activity(s.Snap.ToolTimes, s.Now)
	rows = append(rows, nil,
		label("activity", fmt.Sprintf("%d/min", counts[len(counts)-1]), RoleBold),
		func(y int) { c.Spark(x, y, counts, role, RoleTrack) },
		nil)
	for i, h := range hookStates {
		if i > 0 {
			rows = append(rows, func(y int) { c.Set(x, y, '┆', RoleDim) })
		}
		rows = append(rows, func(y int) {
			mark, mr := '◇', RoleText
			if h == s.Hook && !s.Ended {
				mark, mr = '◆', RoleSelected
				c.HLine(r.X+1, r.X+r.W-2, y, ' ', mr)
			}
			c.Set(x, y, mark, mr)
			c.Text(x+2, y, h, mr)
		})
	}
	done, failed := 0, 0
	for _, a := range s.Snap.Agents {
		switch a.Status {
		case session.AgentDone:
			done++
		case session.AgentError:
			failed++
		}
	}
	mark := s.spin()
	if s.Ended {
		mark = '■'
	}
	agents := []seg{
		{fmt.Sprintf("%c%d", mark, s.running()), role},
		{fmt.Sprintf(" ✓%d", done), RoleOK},
		{fmt.Sprintf(" ✗%d", failed), RoleBad},
	}
	rows = append(rows, nil,
		label("turns", fmtThousands(lead.Turns), RoleBold),
		label("tools", fmtThousands(lead.Calls), RoleBold),
		label("tokens", fmtThousands(lead.Tokens), RoleBold),
		label("  in", fmtThousands(lead.TokensIn), RoleText),
		label("  out", fmtThousands(lead.TokensOut), RoleText),
		label("  cache", fmtThousands(lead.TokensCache), RoleText),
		func(y int) {
			c.Text(x, y, "agents", RoleDim)
			n := min(segsLen(agents), w-7)
			c.segs(x+w-n, y, n, agents...)
		})
	if len(s.Snap.Files) > 0 {
		rows = append(rows, nil, func(y int) { c.Text(x, y, "files", RoleDim) })
		for _, f := range s.Snap.Files {
			rows = append(rows, func(y int) { c.textIn(x, y, w, f, RoleText) })
		}
	}
	rows = append(rows, nil, func(y int) { c.Text(x, y, "last:", RoleDim) })
	lines := strings.Split(ansi.Wrap(strings.Join(strings.Fields(lead.LastText), " "), w, ""), "\n")
	if len(lines) > 3 {
		lines[2] += "…"
	}
	for _, ln := range lines[:min(len(lines), 3)] {
		rows = append(rows, func(y int) { c.textIn(x, y, w, ln, RoleText) })
	}
	if lead.AdvisorModel != "" {
		rows = append(rows, nil,
			label("advisor", shortModel(lead.AdvisorModel), ModelRole(lead.AdvisorModel)),
			label("  calls", fmtThousands(lead.AdvisorCalls), RoleBold))
	}
	return rows
}

// drawLead draws the lead box: the model and the effort meter with its level (left out when unknown), the current
// tool and the hook state with the session's age.
func (s State) drawLead(c *Canvas, r Rect) {
	lead := s.Snap.Lead
	role := ModelRole(lead.Model)
	c.Box(r, leadBorder, role)
	row := []seg{{strings.TrimSpace(shortModel(lead.Model) + " · lead"), role}}
	if m := effortMeter(lead.Effort); m != "" {
		row = append(row, seg{" " + m, role}, seg{" " + strings.ToLower(lead.Effort), RoleDim})
	}
	c.centerSegs(r, r.Y+1, row...)
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
		c.Bar(x+9, y, w-14, t.Share, s.Phase, role, RoleTrack)
		c.Text(x+w-4, y, fmt.Sprintf("%3.0f%%", t.Share*100), RoleText)
	}
}

// drawLog draws the session log as a closed box of the log lines from index start on, as many as fit: time, actor
// in its family color and text, failed ones in the bad color. When the log has more lines than fit, the top
// border shows the newest shown line's number and the total (12/87). A focused log has its border and title in
// the accent.
func (s State) drawLog(c *Canvas, r Rect, start int, focused bool) {
	role, title := panelRoles(focused)
	c.titledIn(r, logBorder, "session log", role, title)
	n := max(r.H-2, 0)
	start = min(max(start, 0), len(s.Log))
	lines := s.Log[start:min(start+n, len(s.Log))]
	if len(s.Log) > n {
		pos := fmt.Sprintf(" %d/%d ", start+len(lines), len(s.Log))
		c.Text(r.X+r.W-2-len(pos), r.Y, pos, title)
	}
	aw := s.actorWidth()
	for i, ln := range lines {
		y := r.Y + 1 + i
		x := c.Text(r.X+2, y, ln.At.Format("15:04:05"), RoleDim) + 1
		c.textIn(x, y, aw, ln.Actor, ModelRole(ln.Model))
		text := RoleText
		if ln.Bad {
			text = RoleBad
		}
		c.textIn(x+aw+1, y, r.X+r.W-x-aw-3, ln.Text, text)
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
// the session is live at the right end. Entries that don't fit whole before it are left out, from the end.
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
	x, end := l.Frame.X+1, l.Frame.X+l.Frame.W-1-runeLen(state.s)
	c.Text(end, l.Status, state.s, state.role)
	end -= 2
	for i, e := range entries {
		n := segsLen(e)
		if i > 0 {
			x += 2
		}
		if x+n > end {
			break
		}
		x = c.segs(x, l.Status, n, e...)
	}
}
