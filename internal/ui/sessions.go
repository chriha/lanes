package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/session"
)

// sessionRow is one line of the sessions screen.
type sessionRow struct {
	info   session.Info
	label  string // item ID + title, or the session's item key when no provider has the item
	orphan bool   // the item is in no view
}

// sessionRows builds the sessions screen rows in the order of infos (newest first).
func sessionRows(infos []session.Info, labelOf func(key string) string, inView map[string]bool) []sessionRow {
	var rows []sessionRow
	for _, s := range infos {
		rows = append(rows, sessionRow{s, labelOf(s.ItemKey), !inView[s.ItemKey]})
	}
	return rows
}

func (m *Model) sessionRows() []sessionRow {
	inView := map[string]bool{}
	for _, v := range m.views {
		for _, it := range v.items {
			inView[session.Key(it.ID)] = true
		}
	}
	return sessionRows(m.sessions, m.itemLabel, inView)
}

func (m *Model) selectedSession() string {
	if m.sessCursor < len(m.sessions) {
		return m.sessions[m.sessCursor].Name
	}
	return ""
}

// restoreSessCursor keeps the sessions cursor on the session called name when it survives, else clamps it.
func (m *Model) restoreSessCursor(name string) {
	for i, s := range m.sessions {
		if s.Name == name {
			m.sessCursor = i
			return
		}
	}
	m.sessCursor = max(0, min(m.sessCursor, len(m.sessions)-1))
}

func (m *Model) openSessions() {
	m.mode, m.sessCursor, m.pendingG = modeSessions, 0, false
}

func (m *Model) keySessions(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.notice = ""
	s := k.String()
	g := m.pendingG
	m.pendingG = false
	switch s {
	case "q", "esc", "S", "ctrl+c":
		m.mode = modeNormal
	case "j", "down":
		m.sessCursor++
	case "k", "up":
		m.sessCursor--
	case "g":
		if g {
			m.sessCursor = 0
		} else {
			m.pendingG = true
		}
	case "G":
		m.sessCursor = len(m.sessions) - 1
	case "enter":
		if name := m.selectedSession(); name != "" {
			return m, m.jumpTo(name)
		}
	case "x":
		if m.sessCursor < len(m.sessions) {
			return m, m.killSessions(m.sessions[m.sessCursor : m.sessCursor+1])
		}
	case "X":
		if m.sessCursor < len(m.sessions) {
			return m, m.interruptSessions(m.sessions[m.sessCursor : m.sessCursor+1])
		}
	}
	m.restoreSessCursor("")
	return m, nil
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

func (m *Model) sessionsView() string {
	rows := m.sessionRows()
	h := max(1, m.height-4) // title, blank, blank, footer
	var b strings.Builder
	b.WriteString(sTitle.Render(fmt.Sprintf(" %s — sessions (%d)", app.Name, len(rows))) + "\n\n")
	lines := 0
	if len(rows) == 0 {
		b.WriteString("  " + sDim.Render("no sessions") + "\n")
		lines++
	}
	// ponytail: the window only follows the cursor down; scroll state is not worth a field here
	for i := max(0, m.sessCursor-h+1); i < len(rows) && lines < h; i++ {
		r := rows[i]
		// every segment carries its own background: a nested style's reset would otherwise end the highlight
		st := func(s lipgloss.Style) lipgloss.Style {
			if i == m.sessCursor {
				return s.Background(cSelBg)
			}
			return s
		}
		cur := st(lipgloss.NewStyle()).Render("  ")
		sp := st(lipgloss.NewStyle()).Render("  ")
		badge := r.info.Badge()
		right := st(badgeStyle(badge.Kind)).Render(badge.Text) + sp + st(sDim).Render(age(time.Since(r.info.Created)))
		if r.orphan {
			right += sp + st(sDim).Render("(not in any view)")
		}
		labelW := max(10, m.width-lipgloss.Width(right)-3)
		label := st(sText).Render(fmt.Sprintf("%-*s", labelW, ansi.Truncate(r.label, labelW, "…")))
		b.WriteString(cur + label + st(lipgloss.NewStyle()).Render(" ") + right + "\n")
		lines++
	}
	for ; lines < h; lines++ {
		b.WriteString("\n")
	}
	b.WriteString("\n")
	switch {
	case m.mode == modeConfirm:
		b.WriteString(" " + sWarn.Render(m.confirm.prompt) + sDim.Render("  y/enter yes · n/esc no"))
	case m.notice != "":
		b.WriteString(m.level.style().Render(" " + strings.SplitN(m.notice, "\n", 2)[0]))
	default:
		b.WriteString(sDim.Render(" j/k move · enter jump · x close · q back"))
	}
	return b.String()
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
