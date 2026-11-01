package session

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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
				Task: "Implement ticket 7", Status: AgentRunning, Start: at("00:05"), Tool: "Grep", ToolArg: "func Poll", Ctx: 5},
			{ID: "b1", Parent: "a1", Depth: 2, Name: "reviewer", Type: "general-purpose", Model: "claude-sonnet-5-5[1m]",
				Task: "Review the diff", Status: AgentDone, Start: at("00:08"), End: at("00:14"), Ctx: 25},
			{ID: "c1", Depth: 1, Name: "verifier", Model: "claude-haiku-5-5", Status: AgentRunning, Start: at("00:20"), Ctx: 0},
		},
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
