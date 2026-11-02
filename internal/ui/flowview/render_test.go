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
		ID: "a1", Name: "implementer", Model: "claude-opus-4-1", Effort: "high", Ctx: 42, Task: "slice 2 of #196", Text: "Reading the diff",
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
			[]string{"┏━ implementer ━", "┃ opus 4.1 ▮▮▮▯ high ┃", "┃ ctx", "┃ slice 2 of #196 ┃", "┃ » Reading the diff ┃", "┗━ ◓ 9m00s ▸ Bash go test ━"}, ""},
		{"no text leaves the row empty", with(func(a *session.Agent) { a.Text = "" }), false,
			[]string{"", "", "", "┃ slice 2 of #196 ┃", "┃ ┃"}, "»"},
		{"long text is cut to the box", with(func(a *session.Agent) { a.Text = strings.Repeat("word ", 12) }), false,
			[]string{"", "", "", "", "┃» word word word word word …┃"}, ""},
		{"unknown effort omits the meter", with(func(a *session.Agent) { a.Effort = "" }), false,
			[]string{"", "┃ opus 4.1 ┃"}, "▮"},
		{"running without a tool", with(func(a *session.Agent) { a.Tool, a.ToolArg = "", "" }), false,
			[]string{"", "", "", "", "", "┗━ ◓ running 9m00s ━"}, "▸"},
		{"done", with(func(a *session.Agent) { a.Status, a.End = session.AgentDone, a.Start.Add(time.Minute) }), false,
			[]string{"", "", "", "", "", "┗━ ✓ done 1m00s ━"}, "▸"},
		{"failed", with(func(a *session.Agent) { a.Status, a.End = session.AgentError, a.Start.Add(time.Minute) }), false,
			[]string{"", "", "", "", "", "┗━ ✗ failed 1m00s ━"}, "▸"},
		{"running in an ended session is stopped", base, true,
			[]string{"", "", "", "", "", "┗━ ■ stopped 9m00s ━"}, "◓"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(30, 6, nil)
			s := State{Now: now, Phase: 1, Ended: tt.ended}
			s.drawAgent(c, Rect{0, 0, 30, 6}, tt.agent)
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

func TestAgentBoxTextIsDimOnceFinished(t *testing.T) {
	tests := []struct {
		name   string
		status session.AgentStatus
		want   Role
	}{
		{"running", session.AgentRunning, RoleText},
		{"done", session.AgentDone, RoleDim},
		{"failed", session.AgentError, RoleDim},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(28, 6, nil)
			a := session.Agent{Name: "a", Model: "claude-opus-4-1", Text: "hello", Status: tt.status, Start: time.Now()}
			State{Phase: 1}.drawAgent(c, Rect{0, 0, 28, 6}, a)
			x := strings.Index(rows(c)[4], "»")
			if x < 0 {
				t.Fatalf("no text row:\n%s", c.String())
			}
			if got := c.At(len([]rune(rows(c)[4][:x])), 4).Role; got != tt.want {
				t.Errorf("text role %d, want %d", got, tt.want)
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
		{"known", 50, "ctx ▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬  50%"},
		{"one percent shows", 1, "ctx ▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬   1%"},
		{"unknown", -1, "ctx ▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬   --"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(28, 5, nil)
			State{}.drawAgent(c, Rect{0, 0, 28, 5}, session.Agent{Name: "a", Model: "claude-opus-4-1", Ctx: tt.ctx})
			if got := strings.TrimSpace(strings.Trim(rows(c)[2], "┃")); got != tt.want {
				t.Errorf("ctx row = %q, want %q", got, tt.want)
			}
			if tt.ctx < 0 {
				for i, r := range segmentRoles(c, 2, '▬') {
					if r != RoleTrack {
						t.Errorf("unknown ctx: cell %d role %d, want RoleTrack", i, r)
					}
				}
			}
		})
	}
}

// segmentRoles returns the roles of the n glyph cells of row y, in order.
func segmentRoles(c *Canvas, y int, glyph rune) []Role {
	var out []Role
	for x := 0; x < c.W; x++ {
		if cell := c.At(x, y); cell.R == glyph {
			out = append(out, cell.Role)
		}
	}
	return out
}

// filledCells counts the filled glyph cells of row y.
func filledCells(c *Canvas, y int, glyph rune) int {
	n := 0
	for _, r := range segmentRoles(c, y, glyph) {
		if r != RoleTrack {
			n++
		}
	}
	return n
}

func TestBarsFillTheirRow(t *testing.T) {
	// the narrowest each gets: the session column is always sessionW wide, an agent box at least boxMinW, the
	// tools box leadW. The bar is every cell between the label and the percent, one space on each side.
	wide := 60
	agent := func(w, ctx int) *Canvas {
		c := NewCanvas(w, 5, nil)
		State{}.drawAgent(c, Rect{0, 0, w, 5}, session.Agent{Name: "a", Model: "claude-opus-4-1", Ctx: ctx})
		return c
	}
	col := func(w int) *Canvas {
		c := NewCanvas(w, 37, nil)
		fixtureState().drawSession(c, Rect{0, 0, w, 37})
		return c
	}
	tools := func(w int) *Canvas {
		c := NewCanvas(w, 6, nil)
		fixtureState().drawTools(c, Rect{0, 0, w, 6})
		return c
	}
	tests := []struct {
		name   string
		c      *Canvas
		y      int
		glyph  rune
		cells  int // the bar's width
		filled int
		row    string // the row between the borders
	}{
		{"agent box, narrowest", agent(boxMinW, 50), 2, '▬', 15, 8, "ctx ▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬ 50%"},
		{"agent box, wide", agent(wide, 50), 2, '▬', wide - 4 - 9, (wide - 4 - 9 + 1) / 2, "ctx " + strings.Repeat("▬", wide-4-9) + " 50%"},
		{"agent box, 100%", agent(boxMinW, 100), 2, '▬', 15, 15, "ctx ▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬ 100%"},
		{"session column, narrowest", col(sessionW), 2, '▬', 9, 5, "ctx ▬▬▬▬▬▬▬▬▬ 61%"},
		{"session column, wide", col(wide), 2, '▬', wide - 4 - 9, 29, "ctx " + strings.Repeat("▬", wide-4-9) + " 61%"},
		{"tools box, narrowest", tools(leadW), 2, '■', 18, 8, "Read     ■■■■■■■■■■■■■■■■■■ 42%"},
		{"tools box, wide", tools(wide), 2, '■', wide - 4 - 14, 18, "Read     " + strings.Repeat("■", wide-4-14) + " 42%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := len(segmentRoles(tt.c, tt.y, tt.glyph)); got != tt.cells {
				t.Errorf("bar is %d cells, want %d", got, tt.cells)
			}
			if got := filledCells(tt.c, tt.y, tt.glyph); got != tt.filled {
				t.Errorf("%d cells filled, want %d", got, tt.filled)
			}
			// ┃ at both edges, one blank before the label and one after the percent
			r := []rune(rows(tt.c)[tt.y])
			if got := strings.Join(strings.Fields(string(r[1:len(r)-1])), " "); got != strings.Join(strings.Fields(tt.row), " ") {
				t.Errorf("row = %q, want %q", got, tt.row)
			}
			if string(r[len(r)-3]) == " " || string(r[len(r)-2]) != " " {
				t.Errorf("the percent does not end at the right inner edge: %q", string(r))
			}
		})
	}
}

func TestToolBarIgnoresPhase(t *testing.T) {
	var base string
	for i, tt := range []struct {
		ended bool
		phase int
	}{{false, 0}, {false, 1}, {true, 0}, {true, 1}} {
		st := fixtureState()
		st.Ended, st.Phase = tt.ended, tt.phase
		c := NewCanvas(leadW, 6, nil)
		st.drawTools(c, Rect{0, 0, leadW, 6})
		bar := rows(c)[2]
		if i == 0 {
			base = bar
		}
		if bar != base {
			t.Errorf("ended %v phase %d: bar row %q, want %q", tt.ended, tt.phase, bar, base)
		}
		// top tool Read: 20 of 48 calls, 42% of 18 cells is 8 full cells, role intact in every phase
		if full := filledCells(c, 2, '■'); full != 8 {
			t.Errorf("ended %v phase %d: %d full cells, want 8", tt.ended, tt.phase, full)
		}
	}
}

func TestPercentFollowsTheBar(t *testing.T) {
	tools := NewCanvas(leadW, 6, nil)
	fixtureState().drawTools(tools, Rect{0, 0, leadW, 6})
	// Read: 20 of 48 calls; the % is one space after the bar, inside the border
	if got := strings.TrimSpace(strings.Trim(rows(tools)[2], "┃")); got != "Read     ■■■■■■■■■■■■■■■■■■  42%" {
		t.Errorf("tools row = %q", got)
	}
	agent := NewCanvas(boxMinW, 5, nil)
	State{}.drawAgent(agent, Rect{0, 0, boxMinW, 5}, session.Agent{Name: "a", Model: "claude-opus-4-1", Ctx: 100})
	if got := strings.TrimSpace(strings.Trim(rows(agent)[2], "┃")); got != "ctx ▬▬▬▬▬▬▬▬▬▬▬▬▬▬▬ 100%" {
		t.Errorf("agent ctx row = %q", got)
	}
}

func TestLeadBox(t *testing.T) {
	noEffort := fixtureState()
	noEffort.Snap.Lead.Effort = ""
	tests := []struct {
		name    string
		st      State
		want    string
		without string
	}{
		{"effort meter and level", fixtureState(), "║ opus 4.1 · lead ▮▮▮▯ high ║", ""},
		{"unknown effort omits both", noEffort, "║ opus 4.1 · lead ║", "▮"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(leadW, 5, nil)
			tt.st.drawLead(c, Rect{0, 0, leadW, 5})
			if g := strings.Join(strings.Fields(rows(c)[1]), " "); g != tt.want {
				t.Errorf("lead row = %q, want %q", g, tt.want)
			}
			if tt.without != "" && strings.Contains(c.String(), tt.without) {
				t.Errorf("lead box shows %q:\n%s", tt.without, c.String())
			}
		})
	}
}

func TestLeadBoxColorsTheEffort(t *testing.T) {
	c := NewCanvas(leadW, 5, nil)
	fixtureState().drawLead(c, Rect{0, 0, leadW, 5})
	row := []rune(rows(c)[1])
	for i, r := range row {
		switch {
		case r == '▮' && c.At(i, 1).Role != RoleOpus:
			t.Errorf("meter cell %d role %d, want the lead's family", i, c.At(i, 1).Role)
		case r == 'h' && c.At(i, 1).Role != RoleDim:
			t.Errorf("level cell %d role %d, want dim", i, c.At(i, 1).Role)
		}
	}
}

func TestHeader(t *testing.T) {
	ended := fixtureState()
	ended.Ended = true
	titled := fixtureState()
	titled.Item = "Make the agent view one scrollable dashboard"
	tests := []struct {
		name string
		st   State
		want string
	}{
		{"the item's title", titled, "AGENT VIEW · Make the agent view one scrollable dashboard"},
		{"the session's name without an item title", fixtureState(), "AGENT VIEW · issue-196"},
		{"ended, still no state", ended, "AGENT VIEW · issue-196"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewLayout(90, 24)
			c := NewCanvas(90, 24, nil)
			tt.st.drawHeader(c, l)
			if got := strings.TrimSpace(rows(c)[0]); got != tt.want {
				t.Errorf("header = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHeaderCutsALongTitle(t *testing.T) {
	st := fixtureState()
	st.Item = strings.Repeat("x", 100)
	c := NewCanvas(52, 3, nil)
	st.drawHeader(c, NewLayout(52, 15))
	if got := rows(c)[0]; !strings.HasSuffix(strings.TrimSpace(got), "x…") || len([]rune(got)) != 52 {
		t.Errorf("header = %q, want the title cut with … inside 52 cells", got)
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
		{"a name wider than the default box widens it", 200, 50, "a-rather-long-agent-name-here", "━ a-rather-long-agent-name-here ━", 200},
		{"on a narrow terminal too", 60, 46, "code-reviewer", "━ code-reviewer ━", 60},
		{"a name wider than the row is cut", 90, 46, strings.Repeat(long, 3), "━ " + strings.Repeat("x", 60) + "… ━", 90},
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
			c := NewCanvas(60, 3, nil)
			s.drawLog(c, Rect{0, 0, 60, 3})
			if got := strings.TrimSpace(strings.Trim(rows(c)[1], "│")); got != tt.want {
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
	noCtx := fixtureState()
	noCtx.Snap.Lead.Ctx = -1
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
			0:  "┏━ session ━",
			1:  "┃ issue-196 12m03s ┃",
			2:  "┃ ctx ▬▬▬▬▬▬▬▬▬ 61% ┃",
			3:  "┃ effort ▮▮▮▯ high ┃",
			5:  "┃ activity 3/min ┃",
			6:  "┃ ▃▁▁▁▂▁▁▇▁▃▁▅█▆▄ ┃",
			8:  "┃ ◆ working ┃",
			9:  "┃ ┆ ┃",
			10: "┃ ◇ asking ┃",
			11: "┃ ┆ ┃",
			12: "┃ ◇ waiting ┃",
			14: "┃ turns 12 ┃",
			15: "┃ tools 48 ┃",
			16: "┃ tokens 1,234,567 ┃",
			17: "┃ in 12,345 ┃",
			18: "┃ out 98,765 ┃",
			19: "┃ cache 1,123,457 ┃",
			20: "┃ agents ◓2 ✓1 ✗1 ┃",
			22: "┃ files ┃",
			23: "┃ render_panels.go ┃",
			26: "┃ README.md ┃",
			28: "┃ last: ┃",
			29: "┃ Slice 2 is next: ┃",
			33: "┃ advisor opus 4.1 ┃",
			34: "┃ calls 3 ┃",
			36: "┗━━",
		}, nil},
		{"unknown effort omits its row", noEffort, 37, map[int]string{3: "", 4: "┃ activity 3/min ┃"}, []string{"effort", "▮"}},
		{"unknown context", noCtx, 37, map[int]string{2: "┃ ctx ▬▬▬▬▬▬▬▬▬ -- ┃"}, nil},
		{"no advisor, no advisor rows", noAdvisor, 37, map[int]string{33: "", 34: ""}, []string{"advisor"}},
		{"no files, no files rows", noFiles, 37, map[int]string{22: "┃ last: ┃"}, []string{"files"}},
		{"ended: no checkpoint is active and no agent runs", ended, 37, map[int]string{8: "┃ ◇ working ┃", 20: "┃ agents ■0 ✓1 ✗1 ┃"}, []string{"◆", "◓"}},
		{"double-digit agent counts fit", tenAgents, 37, map[int]string{20: "┃ agents ◓10 ✓1 ✗1 ┃"}, nil},
		{"rows past the bottom are left out", fixtureState(), 10, map[int]string{8: "┃ ◆ working ┃", 9: "┗━━"}, []string{"asking", "last:"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(22, tt.h, nil)
			tt.st.drawSession(c, Rect{0, 0, 22, tt.h})
			got := rows(c)
			for i, w := range tt.want {
				g := strings.Join(strings.Fields(got[i]), " ")
				if w == "" && strings.Trim(g, "┃ ") != "" || w != "" && !strings.Contains(g, w) {
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

func TestHeaderGlowsInTheAccent(t *testing.T) {
	c := NewCanvas(90, 3, nil)
	fixtureState().drawHeader(c, NewLayout(90, 24))
	for _, p := range []Point{{1, 0}, {5, 0}, {0, 1}, {89, 1}} { // "AGENT VIEW" and both ends of the rule
		if got := c.At(p.X, p.Y).Role; got != RoleAccent {
			t.Errorf("header cell %v %q role %d, want the accent", p, c.At(p.X, p.Y).R, got)
		}
	}
}

func TestTracksAreDrawnInTheTrackRole(t *testing.T) {
	// trackRoles collects the roles of the unfilled track cells and ▁ zero sparks of row y.
	trackRoles := func(c *Canvas, y int, empty rune) map[Role]int {
		got := map[Role]int{}
		for x := 0; x < c.W; x++ {
			if cell := c.At(x, y); cell.R == empty {
				got[cell.Role]++
			}
		}
		return got
	}
	agent := NewCanvas(boxMinW, 5, nil)
	State{}.drawAgent(agent, Rect{0, 0, boxMinW, 5}, session.Agent{Name: "a", Model: "claude-opus-4-1", Ctx: 50})
	col := NewCanvas(sessionW, 37, nil)
	fixtureState().drawSession(col, Rect{0, 0, sessionW, 37})
	tools := NewCanvas(leadW, 6, nil)
	fixtureState().drawTools(tools, Rect{0, 0, leadW, 6})
	tests := []struct {
		name  string
		c     *Canvas
		y     int
		empty rune
	}{
		{"session activity sparkline", col, 6, '▁'},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trackRoles(tt.c, tt.y, tt.empty)
			if len(got) != 1 || got[RoleTrack] == 0 {
				t.Errorf("track cells by role = %v, want all in RoleTrack (%d)", got, RoleTrack)
			}
		})
	}
}

func TestBorders(t *testing.T) {
	lead := NewCanvas(leadW, 5, nil)
	fixtureState().drawLead(lead, Rect{0, 0, leadW, 5})
	agent := NewCanvas(boxMinW, 5, nil)
	State{}.drawAgent(agent, Rect{0, 0, boxMinW, 5}, session.Agent{Name: "a", Model: "claude-opus-4-1"})
	tools := NewCanvas(leadW, 6, nil)
	fixtureState().drawTools(tools, Rect{0, 0, leadW, 6})
	col := NewCanvas(sessionW, 37, nil)
	fixtureState().drawSession(col, Rect{0, 0, sessionW, 37})
	tests := []struct {
		name    string
		c       *Canvas
		corners string // top left, top right, bottom left, bottom right
	}{
		{"lead: double", lead, "╔╗╚╝"},
		{"agent box: heavy", agent, "┏┓┗┛"},
		{"tool layer: heavy", tools, "┏┓┗┛"},
		{"session column: heavy", col, "┏┓┗┛"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.c
			got := string([]rune{c.At(0, 0).R, c.At(c.W-1, 0).R, c.At(0, c.H-1).R, c.At(c.W-1, c.H-1).R})
			if got != tt.corners {
				t.Errorf("corners = %q, want %q", got, tt.corners)
			}
		})
	}
}
