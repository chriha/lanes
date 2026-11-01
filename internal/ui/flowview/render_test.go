package flowview

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/session"
)

func TestCenterSanitizesAndFits(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"short text is centered", "ab", "   ab   "},
		{"escape sequences are stripped", "\x1b[31mab\x1b[0m", "   ab   "},
		{"long text is cut with an ellipsis", "abcdefghij", " abcde… "},
		{"wide runes count one cell each", "日本", "   ??   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(8, 1, nil)
			c.center(Rect{0, 0, 8, 1}, 0, tt.in, RoleText)
			if got := c.String(); got != tt.want {
				t.Errorf("center = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAgentBox(t *testing.T) {
	now := time.Date(2026, 11, 1, 14, 30, 0, 0, time.UTC)
	base := session.Agent{
		ID: "a1", Name: "implementer", Model: "claude-opus-4-1", Effort: "high", Ctx: 42, Task: "slice 2 of #196",
		Start: now.Add(-9 * time.Minute), Tool: "Bash", ToolArg: "go test",
	}
	with := func(f func(a *session.Agent)) session.Agent {
		a := base
		f(&a)
		return a
	}
	tests := []struct {
		name    string
		agent   session.Agent
		ended   bool
		want    []string // what each row holds, spaces collapsed; "" skips the row
		without string   // must not appear anywhere in the box
	}{
		{"running", base, false,
			[]string{"╭─ implementer ─", "│ opus 4.1 ▮▮▮▯ high │", "│ ctx", "│ slice 2 of #196 │", "╰─ ◓ 9m00s ▸ Bash go test ─"}, ""},
		{"unknown effort omits the meter", with(func(a *session.Agent) { a.Effort = "" }), false,
			[]string{"", "│ opus 4.1 │"}, "▮"},
		{"running without a tool", with(func(a *session.Agent) { a.Tool, a.ToolArg = "", "" }), false,
			[]string{"", "", "", "", "╰─ ◓ running 9m00s ─"}, "▸"},
		{"done", with(func(a *session.Agent) { a.Status, a.End = session.AgentDone, a.Start.Add(time.Minute) }), false,
			[]string{"", "", "", "", "╰─ ✓ done 1m00s ─"}, "▸"},
		{"failed", with(func(a *session.Agent) { a.Status, a.End = session.AgentError, a.Start.Add(time.Minute) }), false,
			[]string{"", "", "", "", "╰─ ✗ failed 1m00s ─"}, "▸"},
		{"running in an ended session is stopped", base, true,
			[]string{"", "", "", "", "╰─ ■ stopped 9m00s ─"}, "◓"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(30, 5, nil)
			s := State{Now: now, Phase: 1, Ended: tt.ended}
			s.drawAgent(c, Rect{0, 0, 30, 5}, tt.agent)
			got := rows(c)
			for i, w := range tt.want {
				if w == "" {
					continue
				}
				if g := strings.Join(strings.Fields(got[i]), " "); !strings.Contains(g, w) {
					t.Errorf("row %d = %q, want it to hold %q", i, got[i], w)
				}
			}
			if tt.without != "" && strings.Contains(c.String(), tt.without) {
				t.Errorf("box shows %q:\n%s", tt.without, c.String())
			}
		})
	}
}

func TestAgentBoxOfAnEndedSessionIsDim(t *testing.T) {
	a := session.Agent{Name: "a", Model: "claude-opus-4-1", Ctx: 42, Start: time.Now()}
	for _, ended := range []bool{false, true} {
		c := NewCanvas(28, 5, nil)
		State{Ended: ended, Phase: 1}.drawAgent(c, Rect{0, 0, 28, 5}, a)
		want := RoleOpus
		if ended {
			want = RoleDim
		}
		for _, p := range []Point{{0, 0}, {6, 2}} { // the border and the gauge's fill
			if got := c.At(p.X, p.Y).Role; got != want {
				t.Errorf("ended %v: cell %v role %d, want %d", ended, p, got, want)
			}
		}
	}
}

func TestAgentBoxContextGauge(t *testing.T) {
	tests := []struct {
		name string
		ctx  int
		want string
	}{
		{"known", 50, "ctx ███████▒░░░░░░░  50%"},
		{"unknown", -1, "ctx ░░░░░░░░░░░░░░░   --"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(28, 5, nil)
			State{}.drawAgent(c, Rect{0, 0, 28, 5}, session.Agent{Name: "a", Model: "claude-opus-4-1", Ctx: tt.ctx})
			if got := strings.TrimSpace(strings.Trim(rows(c)[2], "│")); got != tt.want {
				t.Errorf("ctx row = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAgentBoxTitleFitsTheName(t *testing.T) {
	long := strings.Repeat("x", 40)
	tests := []struct {
		name   string
		w, h   int
		agent  string
		want   string
		maxBox int
	}{
		{"a name wider than the default box widens it", 200, 50, "a-rather-long-agent-name-here", "─ a-rather-long-agent-name-here ─", 200},
		{"on a narrow terminal too", 60, 46, "code-reviewer", "─ code-reviewer ─", 60},
		{"a name wider than the row is cut", 90, 46, strings.Repeat(long, 3), "─ " + strings.Repeat("x", 60) + "… ─", 90},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(nil)
			m.SetSize(tt.w, tt.h)
			st := fixtureState()
			st.Snap.Agents = []session.Agent{{ID: "a1", Name: tt.agent, Model: "claude-opus-4-1", Start: st.Now, Ctx: -1}}
			m.SetState(st)
			v := ansi.Strip(m.View())
			if !strings.Contains(v, tt.want) {
				t.Errorf("view lacks the title %q:\n%s", tt.want, v)
			}
			if w, _ := lipgloss.Size(v); w != tt.w {
				t.Errorf("view is %d wide, want %d", w, tt.w)
			}
		})
	}
}

func TestLogActorColumn(t *testing.T) {
	at := time.Date(2026, 11, 1, 14, 28, 30, 0, time.UTC)
	tests := []struct {
		name   string
		agents []string
		actor  string
		want   string
	}{
		{"the column fits the longest agent name", []string{"code-reviewer", "x"}, "x", "14:28:30 x             did it"},
		{"a long actor is not cut", nil, "code-reviewer", "14:28:30 code-reviewer did it"},
		{"the lead needs at least its name", nil, "lead", "14:28:30 lead did it"},
		{"names past 20 cells are cut", []string{strings.Repeat("n", 25)}, strings.Repeat("n", 25), "14:28:30 " + strings.Repeat("n", 19) + "… did it"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := State{Log: []LogLine{{At: at, Actor: tt.actor, Text: "did it"}}}
			for _, n := range tt.agents {
				s.Snap.Agents = append(s.Snap.Agents, session.Agent{Name: n})
			}
			c := NewCanvas(60, 2, nil)
			s.drawLog(c, Rect{0, 0, 60, 2})
			if got := strings.TrimSpace(rows(c)[1]); got != tt.want {
				t.Errorf("log line = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatusLine(t *testing.T) {
	live := fixtureState()
	ended := fixtureState()
	ended.Ended = true
	none := fixtureState()
	none.Snap.Agents = nil
	tests := []struct {
		name    string
		w       int
		st      State
		want    []string
		without []string
	}{
		{"live lists each family and the totals", 200, live,
			[]string{"opus [1 running]  sonnet [1 running]  haiku [done]  agents [2/4]  tools [48]  tokens [1,234,567]", "● live"}, nil},
		{"ended reports no running agents", 200, ended,
			[]string{"opus [stopped]", "sonnet [1 failed]", "haiku [done]", "agents [0/4]", "■ ended"}, []string{"running"}},
		{"no agents, no families", 200, none, []string{"agents [0/0]  tools [48]"}, []string{"opus ["}},
		{"entries that don't fit are left out whole", 90, live, []string{"agents [2/4]", "● live"}, []string{"…", "tokens"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewLayout(tt.w, 50)
			c := NewCanvas(tt.w, 50, nil)
			tt.st.drawStatus(c, l)
			line := rows(c)[l.Status]
			for _, w := range tt.want {
				if !strings.Contains(line, w) {
					t.Errorf("status line %q lacks %q", line, w)
				}
			}
			for _, w := range tt.without {
				if strings.Contains(line, w) {
					t.Errorf("status line %q has %q", line, w)
				}
			}
		})
	}
}

func TestStatusLineColorsFamilies(t *testing.T) {
	l := NewLayout(200, 50)
	c := NewCanvas(200, 50, nil)
	fixtureState().drawStatus(c, l)
	line := []rune(rows(c)[l.Status])
	for fam, role := range map[string]Role{"opus": RoleOpus, "sonnet": RoleSonnet, "haiku": RoleHaiku} {
		i := strings.Index(string(line), fam+" [")
		if i < 0 {
			t.Fatalf("status line lacks %s", fam)
		}
		x := len([]rune(string(line)[:i]))
		if got := c.At(x, l.Status).Role; got != role {
			t.Errorf("%s drawn in role %d, want %d", fam, got, role)
		}
	}
}

func TestSessionColumn(t *testing.T) {
	noEffort, noAdvisor, ended, noFiles := fixtureState(), fixtureState(), fixtureState(), fixtureState()
	noEffort.Snap.Lead.Effort = ""
	noAdvisor.Snap.Lead.AdvisorModel = ""
	ended.Ended = true
	noFiles.Snap.Files = nil
	tenAgents := fixtureState()
	for i := 0; i < 8; i++ {
		tenAgents.Snap.Agents = append(tenAgents.Snap.Agents, session.Agent{ID: fmt.Sprint(i)})
	}
	tests := []struct {
		name    string
		st      State
		h       int
		want    map[int]string // row → what it holds, spaces collapsed
		without []string       // must not appear anywhere in the column
	}{
		{"the whole dashboard", fixtureState(), 37, map[int]string{
			0:  "╭─ session ─",
			1:  "│ issue-196 12m03s │",
			2:  "│ ctx █████▓░░░ 61% │",
			3:  "│ effort ▮▮▮▯ high │",
			5:  "│ activity 3/min │",
			6:  "│ ▃▁▁▁▂▁▁▇▁▃▁▅█▆▄ │",
			8:  "│ ◆ working │",
			9:  "│ ┆ │",
			10: "│ ◇ asking │",
			11: "│ ┆ │",
			12: "│ ◇ waiting │",
			14: "│ turns 12 │",
			15: "│ tools 48 │",
			16: "│ tokens 1,234,567 │",
			17: "│ in 12,345 │",
			18: "│ out 98,765 │",
			19: "│ cache 1,123,457 │",
			20: "│ agents ◓2 ✓1 ✗1 │",
			22: "│ files │",
			23: "│ render_panels.go │",
			26: "│ README.md │",
			28: "│ last: │",
			29: "│ Slice 2 is next: │",
			33: "│ advisor opus 4.1 │",
			34: "│ calls 3 │",
			36: "╰──",
		}, nil},
		{"unknown effort omits its row", noEffort, 37, map[int]string{3: "", 4: "│ activity 3/min │"}, []string{"effort", "▮"}},
		{"no advisor, no advisor rows", noAdvisor, 37, map[int]string{33: "", 34: ""}, []string{"advisor"}},
		{"no files, no files rows", noFiles, 37, map[int]string{22: "│ last: │"}, []string{"files"}},
		{"ended: no checkpoint is active and no agent runs", ended, 37, map[int]string{8: "│ ◇ working │", 20: "│ agents ■0 ✓1 ✗1 │"}, []string{"◆", "◓"}},
		{"double-digit agent counts fit", tenAgents, 37, map[int]string{20: "│ agents ◓10 ✓1 ✗1 │"}, nil},
		{"rows past the bottom are left out", fixtureState(), 10, map[int]string{8: "│ ◆ working │", 9: "╰──"}, []string{"asking", "last:"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(22, tt.h, nil)
			tt.st.drawSession(c, Rect{0, 0, 22, tt.h})
			got := rows(c)
			for i, w := range tt.want {
				g := strings.Join(strings.Fields(got[i]), " ")
				if w == "" && strings.Trim(g, "│ ") != "" || w != "" && !strings.Contains(g, w) {
					t.Errorf("row %d = %q, want it to hold %q", i, got[i], w)
				}
			}
			for _, w := range tt.without {
				if strings.Contains(c.String(), w) {
					t.Errorf("column shows %q:\n%s", w, c.String())
				}
			}
		})
	}
}

func TestSessionColumnHighlightsTheHookState(t *testing.T) {
	c := NewCanvas(22, 37, nil)
	fixtureState().drawSession(c, Rect{0, 0, 22, 37})
	for x := 1; x < 21; x++ {
		if got := c.At(x, 8).Role; got != RoleSelected {
			t.Fatalf("cell %d of the working row has role %d, want the selection", x, got)
		}
	}
	if got := c.At(2, 6).Role; got != RoleOpus {
		t.Errorf("sparkline drawn in role %d, want the lead's family", got)
	}
}
