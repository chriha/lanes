package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// copyFixture copies testdata/tail into a temp dir and returns the path of its main transcript.
func copyFixture(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir("testdata/tail", func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel("testdata/tail", p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dst, "sess.jsonl")
}

func at(s string) time.Time {
	tm, err := time.Parse(time.RFC3339, "2026-11-01T10:"+s+"Z")
	if err != nil {
		panic(err)
	}
	return tm
}

var fixtureLead = Lead{
	Model:     "claude-opus-5-5",
	Tool:      "Bash",
	ToolArg:   "make test",
	Turns:     6,
	Calls:     4,
	ToolCalls: map[string]int{"Read": 1, "Agent": 2, "Bash": 1},
	Tokens:    298,
	LastText:  "Waiting for the agents.",

	Effort:       "medium",
	Ctx:          1,
	TokensIn:     22,
	TokensOut:    56,
	TokensCache:  15820,
	AdvisorModel: "claude-opus-5-5",
	AdvisorCalls: 1,
}

func TestTailerFixture(t *testing.T) {
	got, err := NewTailer(copyFixture(t)).Poll()
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	want := Snapshot{
		Lead: fixtureLead,
		Agents: []Agent{
			{ID: "a2", Depth: 1, Name: "Explore", Type: "Explore", Model: "haiku", Task: "Find callers",
				Status: AgentError, Ctx: -1},
			{ID: "a1", Depth: 1, Name: "implementer", Type: "general-purpose", Model: "claude-opus-5-5",
				Task: "Implement ticket 7", Status: AgentRunning, Start: at("00:05"), Tool: "Grep", ToolArg: "func Poll", Ctx: 5,
				Effort: "low", Tokens: 1094, Calls: 5},
			{ID: "b1", Parent: "a1", Depth: 2, Name: "reviewer", Type: "general-purpose", Model: "claude-sonnet-5-5[1m]",
				Task: "Review the diff", Text: "none", Status: AgentDone, Start: at("00:08"), End: at("00:14"), Ctx: 25, Effort: "xhigh",
				Tokens: 113, Calls: 3},
			{ID: "c1", Depth: 1, Name: "verifier", Model: "claude-haiku-5-5", Text: "ok", Status: AgentRunning, Start: at("00:20"), Ctx: 0,
				Tokens: 11, Calls: 1},
		},
		ToolTimes: []time.Time{at("00:03"), at("00:05"), at("00:05"), at("00:06"), at("00:06"), at("00:06"),
			at("00:06.5"), at("00:07"), at("00:09"), at("00:10"), at("00:10"), at("00:11"), at("00:20")},
		Files: []string{"design.md", "x.go", "a.go", "n.ipynb"},
	}
	if !reflect.DeepEqual(got.ToolTimes, want.ToolTimes) {
		t.Errorf("tool times =\n%v\nwant\n%v", got.ToolTimes, want.ToolTimes)
	}
	if !reflect.DeepEqual(got.Files, want.Files) {
		t.Errorf("files = %q, want %q", got.Files, want.Files)
	}
	if !reflect.DeepEqual(got.Lead, want.Lead) {
		t.Errorf("lead =\n%+v\nwant\n%+v", got.Lead, want.Lead)
	}
	if len(got.Agents) != len(want.Agents) {
		t.Fatalf("agents = %+v, want %d", got.Agents, len(want.Agents))
	}
	for i := range want.Agents {
		if !reflect.DeepEqual(got.Agents[i], want.Agents[i]) {
			t.Errorf("agent %d =\n%+v\nwant\n%+v", i, got.Agents[i], want.Agents[i])
		}
	}
}

func TestTailerReadsIncrementally(t *testing.T) {
	full, err := os.ReadFile("testdata/tail/sess.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	mid := bytes.Index(full, []byte(`"m4"`)) // inside a line
	tests := []struct {
		name string
		cut  int
	}{
		{"empty first", 0},
		{"mid line", mid},
		{"line boundary", mid + bytes.IndexByte(full[mid:], '\n') + 1},
		{"one byte short", len(full) - 1},
		{"all at once", len(full)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "s.jsonl")
			if err := os.WriteFile(path, full[:tt.cut], 0o600); err != nil {
				t.Fatal(err)
			}
			tl := NewTailer(path)
			if _, err := tl.Poll(); err != nil {
				t.Fatalf("first Poll: %v", err)
			}
			f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.Write(full[tt.cut:]); err != nil {
				t.Fatal(err)
			}
			f.Close()
			got, err := tl.Poll()
			if err != nil {
				t.Fatalf("second Poll: %v", err)
			}
			if !reflect.DeepEqual(got.Lead, fixtureLead) {
				t.Errorf("lead =\n%+v\nwant\n%+v", got.Lead, fixtureLead)
			}
			again, _ := tl.Poll()
			if !reflect.DeepEqual(again.Lead, fixtureLead) {
				t.Errorf("poll without new bytes changed the lead: %+v", again.Lead)
			}
		})
	}
}

func TestTailerHalfWrittenLastLine(t *testing.T) {
	full, err := os.ReadFile("testdata/tail/sess.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, full[:len(full)-10], 0o600); err != nil {
		t.Fatal(err)
	}
	tl := NewTailer(path)
	got, err := tl.Poll()
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if got.Lead.Turns != 5 || got.Lead.Tool != "" || got.Lead.Calls != 3 {
		t.Errorf("with a half-written line: turns %d, tool %q, calls %d; want 5, none, 3",
			got.Lead.Turns, got.Lead.Tool, got.Lead.Calls)
	}
	if err := os.WriteFile(path, full, 0o600); err != nil {
		t.Fatal(err)
	}
	got, _ = tl.Poll()
	if !reflect.DeepEqual(got.Lead, fixtureLead) {
		t.Errorf("after the line completed: %+v", got.Lead)
	}
}

func TestTailerMissingFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")
	tl := NewTailer(path)
	got, err := tl.Poll()
	if err != nil {
		t.Fatalf("Poll on missing files: %v", err)
	}
	if got.Lead.Turns != 0 || len(got.Agents) != 0 {
		t.Errorf("snapshot of nothing = %+v", got)
	}
	src := copyFixture(t)
	b, _ := os.ReadFile(src)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(filepath.Dir(src), "sess"), filepath.Join(dir, "sess")); err != nil {
		t.Fatal(err)
	}
	got, err = tl.Poll()
	if err != nil {
		t.Fatalf("Poll after files appeared: %v", err)
	}
	if got.Lead.Turns != fixtureLead.Turns || len(got.Agents) != 4 {
		t.Errorf("after files appeared: turns %d, agents %d", got.Lead.Turns, len(got.Agents))
	}
}

func TestTailerMetaRewritten(t *testing.T) {
	path := copyFixture(t)
	tl := NewTailer(path)
	if _, err := tl.Poll(); err != nil {
		t.Fatal(err)
	}
	meta := filepath.Join(filepath.Dir(path), "sess", "subagents", "agent-a1.meta.json")
	b := []byte(`{"agentType":"general-purpose","description":"Implement ticket 7","toolUseId":"tA1","spawnDepth":1,"finished":true}`)
	if err := os.WriteFile(meta, b, 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(meta, later, later); err != nil {
		t.Fatal(err)
	}
	got, err := tl.Poll()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range got.Agents {
		if a.ID != "a1" {
			continue
		}
		if a.Status != AgentDone || !a.End.Equal(at("00:07")) || a.Tool != "" {
			t.Errorf("finished agent = status %v, end %v, tool %q; want done at 10:00:07 without tool", a.Status, a.End, a.Tool)
		}
		return
	}
	t.Error("agent a1 missing")
}

func TestAgentName(t *testing.T) {
	tests := []struct {
		name, meta, prompt, want string
	}{
		{"meta name wins", `{"name":"lead-reviewer","agentType":"Explore"}`, "Read and follow /p/briefs/implementer.md", "lead-reviewer"},
		{"brief basename", `{"agentType":"general-purpose"}`, "Read and follow /p/briefs/triager.md\n\nid: 1", "triager"},
		{"agent type", `{"agentType":"Explore"}`, "Find the callers of Poll", "Explore"},
		{"id as last resort", `{}`, "", "x9"},
		{"corrupt meta", `{"name":`, "Read and follow briefs/verifier.md", "verifier"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			sub := filepath.Join(dir, "s", "subagents")
			if err := os.MkdirAll(sub, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(sub, "agent-x9.meta.json"), []byte(tt.meta), 0o600); err != nil {
				t.Fatal(err)
			}
			line := `{"type":"user","message":{"content":` + quote(tt.prompt) + `}}` + "\n"
			if err := os.WriteFile(filepath.Join(sub, "agent-x9.jsonl"), []byte(line), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := NewTailer(filepath.Join(dir, "s.jsonl")).Poll()
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Agents) != 1 || got.Agents[0].Name != tt.want {
				t.Errorf("agents = %+v, want one named %q", got.Agents, tt.want)
			}
		})
	}
}

func TestAgentTask(t *testing.T) {
	tests := []struct {
		name, meta, prompt, want string
	}{
		{"meta description wins", `{"description":"Find callers"}`, "Do something else.", "Find callers"},
		{"first sentence", `{}`, "Find the callers of Poll. Then report.", "Find the callers of Poll."},
		{"brief line skipped", `{}`, "Read and follow /p/briefs/implementer.md\n\nImplement ticket 7. More.", "Implement ticket 7."},
		{"only a brief line", `{}`, "Read and follow /p/briefs/implementer.md", ""},
		{"no prompt", `{}`, "", ""},
		{"first line without a stop", `{}`, "id: 216\nrepo: /x", "id: 216"},
		{"cut at 40 characters", `{}`, "Investigate why the transcript reader drops the last line of every file", "Investigate why the transcript reader d…"},
		{"cut counts characters", `{}`, strings.Repeat("ä", 50), strings.Repeat("ä", 39) + "…"},
		{"exactly 40 characters stays", `{}`, strings.Repeat("a", 40), strings.Repeat("a", 40)},
		{"injected text is no prompt", `{}`, "<system-reminder>x</system-reminder>", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pollAgent(t, tt.meta, `{"type":"user","message":{"content":`+quote(tt.prompt)+`}}`)
			if got.Task != tt.want {
				t.Errorf("Task = %q, want %q", got.Task, tt.want)
			}
		})
	}
}

func TestAgentText(t *testing.T) {
	text := func(s string) string {
		return `{"type":"assistant","message":{"id":"m` + fmt.Sprint(len(s)) + `","content":[{"type":"text","text":` + quote(s) + `}]}}`
	}
	tests := []struct {
		name  string
		lines []string
		want  string
	}{
		{"none", []string{`{"type":"user","message":{"content":"hi"}}`}, ""},
		{"newest text", []string{text("first"), text("second one")}, "second one"},
		{"first line only", []string{text("Line one\nline two")}, "Line one"},
		{"leading blank lines skipped", []string{text("\n\n  Hello\nthere")}, "Hello"},
		{"cut at 60 characters", []string{text(strings.Repeat("word ", 20))}, strings.TrimSpace(strings.Repeat("word ", 12)) + "…"},
		{"a tool call keeps the text", []string{text("Looking"), toolLine("00:01", "Read", `{"file_path":"/a.go"}`)}, "Looking"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pollAgent(t, `{}`, tt.lines...); got.Text != tt.want {
				t.Errorf("Text = %q, want %q", got.Text, tt.want)
			}
		})
	}
}

func TestAgentTokensAndCalls(t *testing.T) {
	const (
		use1  = `{"type":"assistant","message":{"id":"m1","usage":{"input_tokens":1,"output_tokens":2,"cache_read_input_tokens":500,"cache_creation_input_tokens":30}}}`
		use1b = `{"type":"assistant","message":{"id":"m1","usage":{"input_tokens":1,"output_tokens":40,"cache_read_input_tokens":500,"cache_creation_input_tokens":30}}}`
		bare1 = `{"type":"assistant","message":{"id":"m1"}}`
		use2  = `{"type":"assistant","message":{"id":"m2","usage":{"input_tokens":4,"output_tokens":6,"cache_creation_input_tokens":10}}}`
		synth = `{"type":"assistant","message":{"id":"syn","model":"<synthetic>","usage":{"input_tokens":900}}}`
		user  = `{"type":"user","message":{"content":"go"}}`
	)
	tests := []struct {
		name          string
		lines         []string
		tokens, calls int
	}{
		{"no assistant messages", []string{user}, 0, 0},
		{"input, cache write and output count; cache reads don't", []string{use1}, 33, 0},
		{"later usage of a message replaces the earlier", []string{use1, use1b}, 71, 0},
		{"a later line without usage keeps the counted usage", []string{use1, bare1}, 33, 0},
		{"a usage line after a usage-less one still replaces", []string{use1, bare1, use1b}, 71, 0},
		{"messages sum", []string{use1b, use2}, 91, 0},
		{"tool calls count over lines", []string{use2, toolLine("00:01", "Read", `{}`), toolLine("00:02", "Bash", `{}`)}, 20, 2},
		{"synthetic messages are ignored", []string{use2, synth}, 20, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pollAgent(t, `{}`, tt.lines...)
			if got.Tokens != tt.tokens || got.Calls != tt.calls {
				t.Errorf("tokens, calls = %d, %d; want %d, %d", got.Tokens, got.Calls, tt.tokens, tt.calls)
			}
		})
	}
}

// pollAgent polls a session with one agent x9 with the given meta and transcript lines and returns the agent.
func pollAgent(t *testing.T, meta string, lines ...string) Agent {
	t.Helper()
	dir := t.TempDir()
	path := writeSession(t, dir, nil, map[string][]string{"x9": lines})
	if err := os.WriteFile(filepath.Join(dir, "s", "subagents", "agent-x9.meta.json"), []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := NewTailer(path).Poll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Agents) != 1 {
		t.Fatalf("agents = %+v, want one", got.Agents)
	}
	return got.Agents[0]
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestContextWindow(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"claude-opus-5-5", 200_000},
		{"claude-sonnet-5-5[1m]", 1_000_000},
		{"opus", 200_000},
		{"", 200_000},
	}
	for _, tt := range tests {
		if got := ContextWindow(tt.model); got != tt.want {
			t.Errorf("ContextWindow(%q) = %d, want %d", tt.model, got, tt.want)
		}
	}
}

func TestClaudeTranscript(t *testing.T) {
	stateEnv(t)
	t.Setenv("LANES_SESSION", "fix~1")
	if got := ClaudeTranscript("fix~1"); got != "" {
		t.Errorf("without sidecar = %q", got)
	}
	feedStdin(t, `{"session_id":"abc","transcript_path":"/t/abc.jsonl"}`)
	RunHook("Stop")
	if got := ClaudeTranscript("fix~1"); got != "/t/abc.jsonl" {
		t.Errorf("ClaudeTranscript = %q, want /t/abc.jsonl", got)
	}
}

// pollLines writes lines as a transcript and returns the snapshot of one Poll.
func pollLines(t *testing.T, tl *Tailer, path string, lines ...string) Snapshot {
	t.Helper()
	var b []byte
	for _, l := range lines {
		b = append(b, l+"\n"...)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := tl.Poll()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTailerLeadEdgeCases(t *testing.T) {
	const (
		read1 = `{"type":"assistant","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/a/x.go"}}]}}`
		grep2 = `{"type":"assistant","message":{"id":"m1","content":[{"type":"tool_use","id":"t2","name":"Grep","input":{"pattern":"Poll"}}]}}`
		done2 = `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t2"}]}}`
		done1 = `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1"}]}}`
		use1  = `{"type":"assistant","message":{"id":"m1","usage":{"input_tokens":1,"output_tokens":2}}}`
		use1b = `{"type":"assistant","message":{"id":"m1","usage":{"input_tokens":1,"output_tokens":50}}}`
	)
	tests := []struct {
		name            string
		lines           []string
		wantTool, wantA string
		wantTokens      int
	}{
		{"parallel calls: newest runs", []string{read1, grep2}, "Grep", "Poll", 0},
		{"newest done: the older still runs", []string{read1, grep2, done2}, "Read", "x.go", 0},
		{"all done", []string{read1, grep2, done2, done1}, "", "", 0},
		{"later usage of a message replaces the earlier", []string{use1, use1b}, "", "", 51},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "s.jsonl")
			got := pollLines(t, NewTailer(path), path, tt.lines...).Lead
			if got.Tool != tt.wantTool || got.ToolArg != tt.wantA || got.Tokens != tt.wantTokens {
				t.Errorf("tool %q %q, tokens %d; want %q %q, %d", got.Tool, got.ToolArg, got.Tokens, tt.wantTool, tt.wantA, tt.wantTokens)
			}
		})
	}
}

func TestTailerRewrittenTranscriptStartsOver(t *testing.T) {
	const msg = `{"type":"assistant","message":{"id":"%s","content":[{"type":"text","text":"%s"}]}}`
	path := filepath.Join(t.TempDir(), "s.jsonl")
	tl := NewTailer(path)
	pollLines(t, tl, path, fmt.Sprintf(msg, "m1", "one"), fmt.Sprintf(msg, "m2", "two"))
	got := pollLines(t, tl, path, fmt.Sprintf(msg, "n1", "new"))
	if got.Lead.Turns != 1 || got.Lead.LastText != "new" {
		t.Errorf("after rewrite: turns %d, text %q; want 1, new", got.Lead.Turns, got.Lead.LastText)
	}
}

func TestAgentBriefAfterInjectedText(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "s", "subagents")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := `{"type":"user","message":{"content":"<system-reminder>context</system-reminder>"}}` + "\n" +
		`{"type":"user","message":{"content":[{"type":"text","text":"Read and follow /p/briefs/verifier.md"}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(sub, "agent-x.jsonl"), []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := NewTailer(filepath.Join(dir, "s.jsonl")).Poll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Agents) != 1 || got.Agents[0].Name != "verifier" {
		t.Errorf("agents = %+v, want one named verifier", got.Agents)
	}
}

func TestCtxPercent(t *testing.T) {
	tests := []struct {
		name   string
		tokens int
		model  string
		want   int
	}{
		{"200k window", 50_000, "claude-opus-5-5", 25},
		{"1m suffix", 500_000, "claude-opus-5-5[1m]", 50},
		{"over 200k without suffix: 1M window", 300_000, "claude-opus-5-5", 30},
		{"never over 100", 1_200_000, "claude-opus-5-5", 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ctxPercent(tt.tokens, tt.model); got != tt.want {
				t.Errorf("ctxPercent(%d, %q) = %d, want %d", tt.tokens, tt.model, got, tt.want)
			}
		})
	}
}

func TestTailerLeadDashboardFields(t *testing.T) {
	const (
		bare     = `{"type":"assistant","message":{"id":"m1","content":[{"type":"text","text":"hi"}]}}`
		high     = `{"type":"assistant","effort":"high","message":{"id":"m1","usage":{"input_tokens":10,"output_tokens":20}}}`
		low      = `{"type":"assistant","effort":"low","perTurnEffort":"low","message":{"id":"m2"}}`
		noEffort = `{"type":"assistant","message":{"id":"m3"}}`
		use1     = `{"type":"assistant","message":{"id":"m1","usage":{"input_tokens":1,"output_tokens":2,"cache_read_input_tokens":5,"cache_creation_input_tokens":3}}}`
		use1b    = `{"type":"assistant","message":{"id":"m1","usage":{"input_tokens":1,"output_tokens":50,"cache_read_input_tokens":5,"cache_creation_input_tokens":3}}}`
		use2     = `{"type":"assistant","message":{"id":"m2","usage":{"input_tokens":4,"output_tokens":1,"cache_read_input_tokens":100000,"cache_creation_input_tokens":0}}}`
		adv1     = `{"type":"assistant","advisorModel":"claude-opus-5-5","message":{"id":"m1","content":[{"type":"server_tool_use","id":"s1","name":"advisor","input":{}}]}}`
		adv2     = `{"type":"assistant","advisorModel":"claude-opus-5-6","message":{"id":"m2","content":[{"type":"server_tool_use","id":"s2","name":"advisor","input":{}},{"type":"server_tool_use","id":"s3","name":"web_search","input":{}}]}}`
		advNone  = `{"type":"assistant","message":{"id":"m3","content":[{"type":"text","text":"done"}]}}`
		synth    = `{"type":"assistant","effort":"max","advisorModel":"x","message":{"id":"syn","model":"<synthetic>","usage":{"input_tokens":9}}}`
	)
	type fields struct {
		effort                string
		ctx, in, out, cache   int
		advisor               string
		advisorCalls, outTool int
	}
	tests := []struct {
		name  string
		lines []string
		want  fields
	}{
		{"lines without the fields", []string{bare}, fields{ctx: -1}},
		{"effort of the newest message wins", []string{high, low}, fields{effort: "low", ctx: 0, in: 10, out: 20}},
		{"a message without effort keeps the last one", []string{high, noEffort}, fields{effort: "high", ctx: 0, in: 10, out: 20}},
		{"token split: later usage of a message replaces the earlier", []string{use1, use1b}, fields{ctx: 0, in: 1, out: 50, cache: 8}},
		{"token split sums messages; ctx of the newest", []string{use1, use2}, fields{ctx: 50, in: 5, out: 3, cache: 100008}},
		{"a message without usage keeps the last ctx", []string{use2, noEffort}, fields{ctx: 50, in: 4, out: 1, cache: 100000}},
		{"advisor model newest, calls counted", []string{adv1, adv2, advNone}, fields{ctx: -1, advisor: "claude-opus-5-6", advisorCalls: 2}},
		{"synthetic messages are ignored", []string{bare, synth}, fields{ctx: -1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "s.jsonl")
			l := pollLines(t, NewTailer(path), path, tt.lines...).Lead
			got := fields{l.Effort, l.Ctx, l.TokensIn, l.TokensOut, l.TokensCache, l.AdvisorModel, l.AdvisorCalls, l.Calls}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// writeSession writes the lead's transcript lines and each agent's transcript lines under dir and returns the
// lead's path.
func writeSession(t *testing.T, dir string, lead []string, agents map[string][]string) string {
	t.Helper()
	join := func(lines []string) []byte {
		var b []byte
		for _, l := range lines {
			b = append(b, l+"\n"...)
		}
		return b
	}
	path := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(path, join(lead), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "s", "subagents")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for id, lines := range agents {
		if err := os.WriteFile(filepath.Join(sub, "agent-"+id+".jsonl"), join(lines), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// toolLine is an assistant line at 10:<ts> with one tool call; ts "" leaves the timestamp out.
func toolLine(ts, name, input string) string {
	stamp := ""
	if ts != "" {
		stamp = `"timestamp":"2026-11-01T10:` + ts + `Z",`
	}
	return `{"type":"assistant",` + stamp + `"message":{"id":"` + name + ts + `","content":[{"type":"tool_use","id":"t` + ts +
		`","name":"` + name + `","input":` + input + `}]}}`
}

func TestTailerToolTimes(t *testing.T) {
	text := func(ts string) string {
		return `{"type":"assistant","timestamp":"2026-11-01T10:` + ts + `Z","message":{"id":"x` + ts + `","content":[{"type":"text","text":"hi"}]}}`
	}
	tests := []struct {
		name   string
		lead   []string
		agents map[string][]string
		want   []time.Time
	}{
		{"none", []string{text("00:00")}, nil, nil},
		{"older than the window before the newest line dropped, the boundary kept",
			[]string{toolLine("00:00", "Read", `{}`), toolLine("05:00", "Read", `{}`), toolLine("20:00", "Bash", `{}`)}, nil,
			[]time.Time{at("05:00"), at("20:00")}},
		{"a newer non-tool line moves the window",
			[]string{toolLine("05:00", "Read", `{}`), toolLine("10:00", "Read", `{}`), text("21:00")}, nil,
			[]time.Time{at("10:00")}},
		{"agents' calls merged in order; an agent's newer line moves the window",
			[]string{toolLine("00:00", "Read", `{}`), toolLine("12:00", "Read", `{}`)},
			map[string][]string{"a": {toolLine("11:00", "Grep", `{}`), text("26:00")}},
			[]time.Time{at("11:00"), at("12:00")}},
		{"a call without a timestamp is left out",
			[]string{toolLine("", "Read", `{}`), toolLine("01:00", "Read", `{}`)}, nil,
			[]time.Time{at("01:00")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewTailer(writeSession(t, t.TempDir(), tt.lead, tt.agents)).Poll()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.ToolTimes, tt.want) {
				t.Errorf("tool times = %v, want %v", got.ToolTimes, tt.want)
			}
		})
	}
}

func TestTailerFiles(t *testing.T) {
	path := func(p string) string { return `{"file_path":"` + p + `"}` }
	tests := []struct {
		name   string
		lead   []string
		agents map[string][]string
		want   []string
	}{
		{"none", []string{toolLine("00:00", "Read", path("/a/r.go"))}, nil, nil},
		{"Edit, Write and NotebookEdit, newest first, base names",
			[]string{toolLine("00:01", "Edit", path("/a/e.go")), toolLine("00:02", "Write", path("/a/w.go")),
				toolLine("00:03", "NotebookEdit", `{"notebook_path":"/a/n.ipynb"}`), toolLine("00:04", "Read", path("/a/r.go"))}, nil,
			[]string{"n.ipynb", "w.go", "e.go"}},
		{"a file touched again moves to the front, once",
			[]string{toolLine("00:01", "Edit", path("/a/e.go")), toolLine("00:02", "Write", path("/a/w.go")),
				toolLine("00:03", "Edit", path("/a/e.go"))}, nil,
			[]string{"e.go", "w.go"}},
		{"at most four",
			[]string{toolLine("00:01", "Edit", path("/a/1.go")), toolLine("00:02", "Edit", path("/a/2.go")),
				toolLine("00:03", "Edit", path("/a/3.go")), toolLine("00:04", "Edit", path("/a/4.go")),
				toolLine("00:05", "Edit", path("/a/5.go"))}, nil,
			[]string{"5.go", "4.go", "3.go", "2.go"}},
		{"lead and agents merged by time",
			[]string{toolLine("00:01", "Edit", path("/a/lead1.go")), toolLine("00:04", "Edit", path("/a/lead4.go"))},
			map[string][]string{
				"a": {toolLine("00:02", "Write", path("/a/a2.go")), toolLine("00:05", "Edit", path("/a/a5.go"))},
				"b": {toolLine("00:03", "Edit", path("/a/lead1.go"))},
			},
			[]string{"a5.go", "lead4.go", "lead1.go", "a2.go"}},
		{"calls without a path are left out",
			[]string{toolLine("00:01", "Edit", `{}`), toolLine("00:02", "Write", path("  ")), toolLine("00:03", "Edit", `"bad"`)}, nil,
			nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewTailer(writeSession(t, t.TempDir(), tt.lead, tt.agents)).Poll()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Files, tt.want) {
				t.Errorf("files = %q, want %q", got.Files, tt.want)
			}
		})
	}
}

// fmtEvents is one line per event: time (-- when unknown), actor, model, kind, text and a ✗ for a failure.
func fmtEvents(evs []LogEvent) string {
	var b strings.Builder
	for _, e := range evs {
		ts := "--"
		if !e.At.IsZero() {
			ts = e.At.Format("04:05.0")
		}
		mark := ""
		if e.Failed {
			mark = " ✗"
		}
		fmt.Fprintf(&b, "%s %s %s %d %s%s\n", ts, e.Actor, e.Model, e.Kind, e.Text, mark)
	}
	return b.String()
}

// fixtureLeadEvents are the events of the fixture's lead transcript; the Bash call has no result yet.
const fixtureLeadEvents = `00:02.0 lead claude-opus-5-5 1 Looking at the ticket.
00:03.0 lead claude-opus-5-5 0 Read implementer.md
00:05.0 lead claude-opus-5-5 0 Agent Implement ticket 7
00:05.0 lead claude-opus-5-5 0 Agent Find callers ✗
00:08.0 lead claude-opus-5-5 1 Waiting for the agents.
`

func TestTailerEvents(t *testing.T) {
	tl := NewTailer(copyFixture(t))
	got, err := tl.Poll()
	if err != nil {
		t.Fatal(err)
	}
	// Ordered by time; at equal times the lead first, then spawns, then the agents' own events, then ends. The
	// Explore agent has no transcript: its spawn and failure have no time. A call the model went on without is logged as interrupted.
	want := `-- lead claude-opus-5-5 2 spawned Explore
-- Explore haiku 4 failed ✗
00:02.0 lead claude-opus-5-5 1 Looking at the ticket.
00:03.0 lead claude-opus-5-5 0 Read implementer.md
00:05.0 lead claude-opus-5-5 0 Agent Implement ticket 7
00:05.0 lead claude-opus-5-5 0 Agent Find callers ✗
00:05.0 lead claude-opus-5-5 2 spawned implementer
00:06.0 implementer claude-opus-5-5 0 Write old.go
00:06.0 implementer claude-opus-5-5 0 Agent Review the diff (interrupted)
00:06.0 implementer claude-opus-5-5 0 Edit x.go
00:06.5 implementer claude-opus-5-5 0 NotebookEdit
00:08.0 lead claude-opus-5-5 1 Waiting for the agents.
00:08.0 implementer claude-opus-5-5 2 spawned reviewer
00:09.0 reviewer claude-sonnet-5-5[1m] 0 Read a.go
00:10.0 reviewer claude-sonnet-5-5[1m] 0 Write a.go
00:11.0 reviewer claude-sonnet-5-5[1m] 0 Edit x.go
00:14.0 reviewer claude-sonnet-5-5[1m] 1 none
00:14.0 reviewer claude-sonnet-5-5[1m] 3 done
00:20.0 lead claude-opus-5-5 2 spawned verifier
00:20.0 verifier claude-haiku-5-5 0 Write design.md
00:21.0 verifier claude-haiku-5-5 1 ok
`
	if s := fmtEvents(got.Events); s != want {
		t.Errorf("events =\n%s\nwant\n%s", s, want)
	}
	again, err := tl.Poll()
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Events) != 0 {
		t.Errorf("a poll without new lines repeated events:\n%s", fmtEvents(again.Events))
	}
}

func TestTailerEventsWithoutResult(t *testing.T) {
	user := func(ts, id string, isErr bool) string {
		return fmt.Sprintf(`{"type":"user","timestamp":"2026-11-01T10:%sZ","message":{"content":[{"type":"tool_result","tool_use_id":"%s","is_error":%t}]}}`, ts, id, isErr)
	}
	text := func(ts string) string {
		return `{"type":"assistant","timestamp":"2026-11-01T10:` + ts + `Z","message":{"id":"x` + ts + `","content":[{"type":"text","text":"moving on"}]}}`
	}
	tests := []struct {
		name string
		lead []string
		want string
	}{
		{"a call the model moved on from is logged as interrupted",
			[]string{toolLine("00:01", "Bash", `{"command":"make"}`), user("00:02", "other", false), text("00:03")},
			"00:01.0 lead  0 Bash make (interrupted)\n00:03.0 lead  1 moving on\n"},
		{"parallel calls whose results are on the way are not interrupted",
			[]string{toolLine("00:01", "Read", `{}`), toolLine("00:02", "Grep", `{}`), user("00:03", "t00:01", false)},
			"00:01.0 lead  0 Read\n"},
		{"a call still running is not logged yet",
			[]string{toolLine("00:01", "Bash", `{"command":"make"}`)}, ""},
		{"a call whose result comes is logged once with its failure",
			[]string{toolLine("00:01", "Bash", `{"command":"make"}`), user("00:02", "t00:01", true), text("00:03")},
			"00:01.0 lead  0 Bash make ✗\n00:03.0 lead  1 moving on\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewTailer(writeSession(t, t.TempDir(), tt.lead, nil)).Poll()
			if err != nil {
				t.Fatal(err)
			}
			if s := fmtEvents(got.Events); s != tt.want {
				t.Errorf("events =\n%s\nwant\n%s", s, tt.want)
			}
		})
	}
}

func TestTailerEventsOnceAcrossPolls(t *testing.T) {
	full, err := os.ReadFile("testdata/tail/sess.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	result := bytes.Index(full, []byte(`"tool_use_id": "tA1"`)) // the call's line read, its result not yet
	for _, cut := range []int{0, result, len(full) - 1, len(full)} {
		t.Run(fmt.Sprint(cut), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "s.jsonl")
			if err := os.WriteFile(path, full[:cut], 0o600); err != nil {
				t.Fatal(err)
			}
			tl := NewTailer(path)
			first, err := tl.Poll()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, full, 0o600); err != nil {
				t.Fatal(err)
			}
			second, err := tl.Poll()
			if err != nil {
				t.Fatal(err)
			}
			if got := fmtEvents(first.Events) + fmtEvents(second.Events); got != fixtureLeadEvents {
				t.Errorf("events over both polls =\n%s\nwant\n%s", got, fixtureLeadEvents)
			}
		})
	}
}

func TestTailerEventsSkipIncompleteLines(t *testing.T) {
	const (
		noName   = `{"type":"assistant","timestamp":"2026-11-01T10:00:01Z","message":{"content":[{"type":"tool_use","id":"t1","input":{}}]}}`
		noNameOK = `{"type":"user","timestamp":"2026-11-01T10:00:02Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1"}]}}`
		noTime   = `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t2","name":"Read","input":{}}]}}`
		noTimeOK = `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t2"}]}}`
		blank    = `{"type":"assistant","timestamp":"2026-11-01T10:00:03Z","message":{"content":[{"type":"text","text":"  \n "}]}}`
		textNoTS = `{"type":"assistant","message":{"content":[{"type":"text","text":"hi"}]}}`
		orphan   = `{"type":"user","timestamp":"2026-11-01T10:00:04Z","message":{"content":[{"type":"tool_result","tool_use_id":"nope"}]}}`
		multi    = `{"type":"assistant","timestamp":"2026-11-01T10:00:05Z","message":{"content":[{"type":"text","text":"\n first line \nsecond"}]}}`
		bash     = `{"type":"assistant","timestamp":"2026-11-01T10:00:06Z","message":{"content":[{"type":"tool_use","id":"t3","name":"Bash","input":{"command":"go test ./...\nmore"}}]}}`
		bashBad  = `{"type":"user","timestamp":"2026-11-01T10:00:09Z","message":{"content":[{"type":"tool_result","tool_use_id":"t3","is_error":true}]}}`
	)
	path := filepath.Join(t.TempDir(), "s.jsonl")
	got := pollLines(t, NewTailer(path), path, noName, noNameOK, noTime, noTimeOK, blank, textNoTS, orphan, multi, bash, bashBad)
	want := "00:05.0 lead  1 first line\n00:06.0 lead  0 Bash go test ./... ✗\n"
	if s := fmtEvents(got.Events); s != want {
		t.Errorf("events =\n%s\nwant\n%s", s, want)
	}
}
