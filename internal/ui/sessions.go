package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/session"
)

// sessionsName is the name of the built-in view that lists every session. It is always the last view, after the
// queue view, is not configurable and is reached with S.
const sessionsName = config.SessionsView

// newSessionsView is the built-in view of every session; it has no items, its rows are m.sessions.
func newSessionsView() *view { return &view{cfg: config.View{Name: sessionsName}, sessions: true} }

// onSessionsTab says whether the current view is the sessions view.
func (m *Model) onSessionsTab() bool { return m.cur < len(m.views) && m.views[m.cur].sessions }

// sessionRow is one line of the sessions view.
type sessionRow struct {
	info   session.Info
	label  string // item ID + title, or the session's item key when no provider has the item
	orphan bool   // the item is in no view
}

// sessionRows builds the sessions view rows in the order of infos (newest first).
func sessionRows(infos []session.Info, labelOf func(key string) string, inView map[string]bool) []sessionRow {
	var rows []sessionRow
	for _, s := range infos {
		rows = append(rows, sessionRow{s, labelOf(s.ItemKey), !inView[s.ItemKey]})
	}
	return rows
}

// sessionRows are the visible rows of the sessions view: every session, filtered by the search query on its label.
func (m *Model) sessionRows() []sessionRow {
	inView := map[string]bool{}
	for _, v := range m.views {
		if v.builtin() {
			continue // the queue view lists queued items, which no configured view need list
		}
		for _, it := range v.items {
			inView[session.Key(it.ID)] = true
		}
	}
	rows := sessionRows(m.sessions, m.itemLabel, inView)
	q := strings.ToLower(m.query)
	if q == "" {
		return rows
	}
	var out []sessionRow
	for _, r := range rows {
		if strings.Contains(strings.ToLower(r.label), q) {
			out = append(out, r)
		}
	}
	return out
}

func (m *Model) selectedSession() string {
	if rows := m.sessionRows(); m.sessCursor < len(rows) {
		return rows[m.sessCursor].info.Name
	}
	return ""
}

// restoreSessCursor keeps the sessions cursor on the session called name when it is still visible, else clamps it.
func (m *Model) restoreSessCursor(name string) {
	rows := m.sessionRows()
	for i, r := range rows {
		if r.info.Name == name {
			m.sessCursor = i
			return
		}
	}
	m.sessCursor = max(0, min(m.sessCursor, len(rows)-1))
}

// sessMarked says whether the session is marked on the sessions view.
func (m *Model) sessMarked(s session.Info) bool {
	created, ok := m.sessMarks[s.Name]
	return ok && created.Equal(s.Created)
}

// sessionTargets are the marked visible sessions, else the one under the cursor (the targets of x and X).
func (m *Model) sessionTargets() []session.Info {
	rows := m.sessionRows()
	var out []session.Info
	for _, r := range rows {
		if m.sessMarked(r.info) {
			out = append(out, r.info)
		}
	}
	if len(out) == 0 && m.sessCursor < len(rows) {
		out = append(out, rows[m.sessCursor].info)
	}
	return out
}

// keepSessMarks drops the marks of sessions that are gone, were restarted under the same name or ended since the
// poll before (prev), so a mark never acts on a session the user did not mark.
func (m *Model) keepSessMarks(prev map[string]session.Info) {
	now := map[string]session.Info{}
	for _, s := range m.sessions {
		now[s.Name] = s
	}
	for name, created := range m.sessMarks {
		s, ok := now[name]
		if !ok || !s.Created.Equal(created) || s.Dead && !prev[name].Dead {
			delete(m.sessMarks, name)
		}
	}
}

// openSessions switches to the sessions view (S, :sessions).
func (m *Model) openSessions() {
	m.switchView(m.viewIndex(func(v *view) bool { return v.sessions }))
}

// sessionsPass says whether the sessions view leaves key s to the list's handling: quitting, switching views,
// the command line, search, help, refresh, the next session asking and the preview. Every other key it does not
// handle itself does nothing there, so item keys and actions never run from it.
func sessionsPass(s string) bool {
	switch s {
	case "ctrl+c", "q", "l", "tab", "right", "h", "shift+tab", "left", "Q", "S", ":", "/", "?", "R", "ctrl+r", "n", "p":
		return true
	}
	return len(s) == 1 && s[0] >= '1' && s[0] <= '9'
}

// keySessions handles key s on the sessions view; false when it leaves s alone.
func (m *Model) keySessions(s string) (tea.Cmd, bool) {
	g := m.pendingG
	m.pendingG = false
	rows := m.sessionRows()
	var cmd tea.Cmd
	switch s {
	case "j", "down":
		m.sessCursor++
	case "k", "up":
		m.sessCursor--
	case "ctrl+d":
		m.sessCursor += m.listHeight() / 2
	case "ctrl+u":
		m.sessCursor -= m.listHeight() / 2
	case "g":
		if g {
			m.sessCursor = 0
		} else {
			m.pendingG = true
		}
	case "G":
		m.sessCursor = len(rows) - 1
	case " ":
		if m.sessCursor < len(rows) {
			s := rows[m.sessCursor].info
			if m.sessMarked(s) {
				delete(m.sessMarks, s.Name)
			} else {
				if m.sessMarks == nil {
					m.sessMarks = map[string]time.Time{}
				}
				m.sessMarks[s.Name] = s.Created
			}
			m.sessCursor++
		}
	case "esc":
		sel := m.selectedSession()
		m.sessMarks = map[string]time.Time{}
		if m.query != "" {
			m.query = ""
			m.search.SetValue("")
			m.rebuild()
		}
		m.restoreSessCursor(sel)
	case "enter":
		if name := m.selectedSession(); name != "" && m.sess != nil {
			cmd = m.jumpTo(name)
		}
	case "x":
		if t := m.sessionTargets(); len(t) > 0 {
			cmd = m.killSessions(t)
		}
	case "X":
		if t := m.sessionTargets(); len(t) > 0 {
			cmd = m.interruptSessions(t)
		}
	case "A":
		if m.sessCursor < len(rows) {
			cmd = m.agentsOfSession(rows[m.sessCursor].info)
		}
	default:
		return nil, false
	}
	m.restoreSessCursor("")
	return cmd, true
}

// sessionsAttention counts the live sessions that are asking or waiting for input.
func (m *Model) sessionsAttention() (n int) {
	for _, s := range m.sessions {
		if !s.Dead && (s.Claude == "asking" || s.Claude == "waiting") {
			n++
		}
	}
	return n
}

// age is a compact duration: 42s, 5m, 3h, 2d.
func age(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// sessionsList is the list area of the sessions view: one row per session, newest first, listHeight lines.
func (m *Model) sessionsList() string {
	rows := m.sessionRows()
	h := m.listHeight()
	if len(rows) > h { // the scrollbar takes the last column
		m.width--
		defer func() { m.width++ }()
	}
	var lines []string
	switch {
	case len(rows) == 0 && m.query != "":
		lines = append(lines, "", "  "+sDim.Render("no sessions match the filter"))
	case len(rows) == 0:
		lines = append(lines, "", "  "+sDim.Render("no sessions"))
	}
	// ponytail: the window only follows the cursor down; scroll state is not worth a field here
	off := max(0, m.sessCursor-h+1)
	for i := off; i < len(rows) && len(lines) < h; i++ {
		lines = append(lines, m.sessionLine(rows[i], i == m.sessCursor))
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return m.withScrollbar(lines, off, len(rows))
}

// sessionLine renders one row of the sessions view, full width.
func (m *Model) sessionLine(r sessionRow, selected bool) string {
	// every segment carries its own background: a nested style's reset would otherwise end the highlight
	st := func(s lipgloss.Style) lipgloss.Style {
		if selected {
			return selectStyle(s, false)
		}
		return s
	}
	cur := st(lipgloss.NewStyle()).Render("  ")
	if m.sessMarked(r.info) {
		cur = st(sMark).Render("▌") + st(lipgloss.NewStyle()).Render(" ")
	}
	sp := st(lipgloss.NewStyle()).Render("  ")
	badge := r.info.Badge()
	right := st(badgeStyle(badge.Kind)).Render(badge.Text) + sp + st(sDim).Render(age(time.Since(r.info.Created)))
	if r.orphan {
		right += sp + st(sDim).Render("(not in any view)")
	}
	// the label gives way first; when even the right side alone is too wide, the row is cut to the width
	labelW := max(0, m.width-lipgloss.Width(right)-3)
	label := st(sText).Render(fmt.Sprintf("%-*s", labelW, ansi.Truncate(r.label, labelW, "…")))
	return ansi.Truncate(cur+label+st(lipgloss.NewStyle()).Render(" ")+right, m.width, "")
}

// nextAttention picks the live session to jump to next: asking before waiting, then oldest first,
// starting after the session called last so repeated presses cycle.
func nextAttention(infos []session.Info, last string) (string, bool) {
	var q []session.Info
	for _, s := range infos {
		if !s.Dead && (s.Claude == "asking" || s.Claude == "waiting") {
			q = append(q, s)
		}
	}
	if len(q) == 0 {
		return "", false
	}
	sort.SliceStable(q, func(i, j int) bool {
		if q[i].Claude != q[j].Claude {
			return q[i].Claude == "asking"
		}
		return q[i].Created.Before(q[j].Created)
	})
	for i, s := range q {
		if s.Name == last {
			return q[(i+1)%len(q)].Name, true
		}
	}
	return q[0].Name, true
}
