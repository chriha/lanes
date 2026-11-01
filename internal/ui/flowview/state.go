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

// topTools returns the lead's n most called tools, most calls first, ties by name. Shares are of the calls per
// tool, not of Calls, which the animation eases.
func (s State) topTools(n int) []toolShare {
	lead := s.Snap.Lead
	names := make([]string, 0, len(lead.ToolCalls))
	total := 0
	for name, k := range lead.ToolCalls {
		names = append(names, name)
		total += k
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := lead.ToolCalls[names[i]], lead.ToolCalls[names[j]]
		return a > b || a == b && names[i] < names[j]
	})
	var out []toolShare
	for _, name := range names[:min(n, len(names))] {
		out = append(out, toolShare{name, float64(lead.ToolCalls[name]) / float64(max(total, 1))})
	}
	return out
}

// running counts the agents still running; none once the session ended, whatever the transcripts say.
func (s State) running() int {
	if s.Ended {
		return 0
	}
	n := 0
	for _, a := range s.Snap.Agents {
		if a.Status == session.AgentRunning {
			n++
		}
	}
	return n
}

// knownFamilies are the model families the status line lists first, in this order.
var knownFamilies = [...]string{"opus", "sonnet", "haiku"}

// family is a model id's family: opus, sonnet or haiku, else its short name without the version.
func family(model string) string {
	m := strings.ToLower(model)
	for _, f := range knownFamilies {
		if strings.Contains(m, f) {
			return f
		}
	}
	name, _, _ := strings.Cut(shortModel(model), " ")
	if name == "" {
		return "other"
	}
	return name
}

// familyStatus is one status line entry: a model family of the agents and how they stand.
type familyStatus struct {
	Family string
	Model  string // a model id of the family, for its color
	Status string // "2 running", "1 failed", "stopped" or "done"
}

// familyStatuses returns an entry per model family of the agents: opus, sonnet and haiku first, then the others
// by name. Running agents win over failed ones, failed over stopped, stopped over done. Once the session ended no
// agent runs: those the transcripts still show running count as stopped.
func (s State) familyStatuses() []familyStatus {
	type counts struct {
		model           string
		run, fail, stop int
	}
	byFam := map[string]*counts{}
	var names []string
	for _, a := range s.Snap.Agents {
		f := family(a.Model)
		c := byFam[f]
		if c == nil {
			c = &counts{model: a.Model}
			byFam[f] = c
			names = append(names, f)
		}
		switch {
		case a.Status == session.AgentRunning && s.Ended:
			c.stop++
		case a.Status == session.AgentRunning:
			c.run++
		case a.Status == session.AgentError:
			c.fail++
		}
	}
	rank := func(f string) int {
		for i, k := range knownFamilies {
			if f == k {
				return i
			}
		}
		return len(knownFamilies)
	}
	sort.Slice(names, func(i, j int) bool {
		ri, rj := rank(names[i]), rank(names[j])
		return ri < rj || ri == rj && names[i] < names[j]
	})
	out := make([]familyStatus, 0, len(names))
	for _, f := range names {
		c := byFam[f]
		status := "done"
		switch {
		case c.run > 0:
			status = fmt.Sprintf("%d running", c.run)
		case c.fail > 0:
			status = fmt.Sprintf("%d failed", c.fail)
		case c.stop > 0:
			status = "stopped"
		}
		out = append(out, familyStatus{Family: f, Model: c.model, Status: status})
	}
	return out
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

// since is the duration from start to end, "" when either is unknown.
func since(start, end time.Time) string {
	if start.IsZero() || end.IsZero() {
		return ""
	}
	return fmtDur(end.Sub(start))
}

// leadState is the lead's state label: the hook state, or ended.
func (s State) leadState() string {
	if s.Ended {
		return "ended"
	}
	return s.Hook
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

// fmtThousands is n with thousands separators: 1,234,567.
func fmtThousands(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0 && s[i-1] != '-'; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
