package flowview

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/chriha/lanes/internal/session"
)

// State is everything one frame of the agent view shows. It is plain data: the caller fills it from the session's
// hook state and transcript snapshot, and the animation steps it.
type State struct {
	Title string // the session's name, shown in the header
	Hook  string // Claude's hook state: working, asking or waiting
	Ended bool   // Claude exited: the frame is the final state
	Start time.Time
	Now   time.Time // the frame's clock
	Phase int       // animation phase: spinner frame and bar shimmer
	Snap  session.Snapshot
	Log   []LogLine // oldest first
}

// LogLine is one line of the session log.
type LogLine struct {
	At    time.Time
	Actor string
	Model string // colors the actor
	Text  string
}

// hookStates are the session column's checkpoints, in order.
var hookStates = [...]string{"working", "asking", "waiting"}

var spinner = [...]rune{'◐', '◓', '◑', '◒'}

func (s State) spin() rune { return spinner[(s.Phase%len(spinner)+len(spinner))%len(spinner)] }

// toolShare is a tool and its share of the lead's tool calls.
type toolShare struct {
	Name  string
	Share float64
}

// topTools returns the lead's n most called tools, most calls first, ties by name.
func (s State) topTools(n int) []toolShare {
	lead := s.Snap.Lead
	names := make([]string, 0, len(lead.ToolCalls))
	for name := range lead.ToolCalls {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := lead.ToolCalls[names[i]], lead.ToolCalls[names[j]]
		return a > b || a == b && names[i] < names[j]
	})
	var out []toolShare
	for _, name := range names[:min(n, len(names))] {
		out = append(out, toolShare{name, float64(lead.ToolCalls[name]) / float64(max(lead.Calls, 1))})
	}
	return out
}

// running counts the agents still running.
func (s State) running() int {
	n := 0
	for _, a := range s.Snap.Agents {
		if a.Status == session.AgentRunning {
			n++
		}
	}
	return n
}

// shortModel turns a model id into family and version: claude-opus-4-1-20250805 → opus 4.1.
func shortModel(id string) string {
	var name, ver []string
	for _, p := range strings.Split(strings.TrimSuffix(id, "[1m]"), "-") {
		switch {
		case p == "" || p == "claude":
		case strings.Trim(p, "0123456789") != "":
			name = append(name, p)
		case len(p) < 8: // longer digit runs are dates
			ver = append(ver, p)
		}
	}
	s := strings.Join(name, "-")
	if len(ver) > 0 {
		s += " " + strings.Join(ver, ".")
	}
	return strings.TrimSpace(s)
}

// fmtDur is a short duration: 42s, 12m03s, 1h02m.
func fmtDur(d time.Duration) string {
	d = max(d, 0).Truncate(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}

// fmtCount is a compact count: 950, 12.3k, 1.2M.
func fmtCount(n int) string {
	switch {
	case n < 1000:
		return fmt.Sprint(n)
	case n < 1_000_000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1e6)
}
