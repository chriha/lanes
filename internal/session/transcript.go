package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// The transcript and the subagent files are Claude Code internals, not a contract: everything here reads only the
// fields it needs, skips lines and fields it doesn't know and degrades when a file is missing.

// AgentStatus is where a subagent of a Claude session stands.
type AgentStatus int

const (
	AgentRunning AgentStatus = iota
	AgentDone
	AgentError
)

// Lead is what the transcript tells about a Claude session's main agent.
type Lead struct {
	Model     string         // model id of the newest assistant message
	Tool      string         // the tool the lead runs right now, "" when none
	ToolArg   string         // a short argument of Tool (file name, pattern, command, …)
	Turns     int            // assistant messages
	Calls     int            // tool calls in total
	ToolCalls map[string]int // tool calls per tool name
	Tokens    int            // input + cache write + output tokens of all assistant messages
	LastText  string         // the newest assistant text
}

// Agent is a subagent of a Claude session, from its meta.json and transcript.
type Agent struct {
	ID      string
	Parent  string // parent agent id, "" for the lead
	Depth   int    // spawn depth, 1 for agents the lead spawned
	Name    string // meta name, else the brief named in the first prompt, else the agent type
	Type    string
	Model   string
	Task    string
	Status  AgentStatus
	Start   time.Time
	End     time.Time // zero while running
	Tool    string    // the tool the agent runs right now, "" when none
	ToolArg string
	Ctx     int // percent of the context window used by the newest assistant message, -1 when unknown
}

// Snapshot is the state of a Claude session derived from its transcripts.
type Snapshot struct {
	Lead   Lead
	Agents []Agent // ordered by start, then id
}

// defaultContextWindow is the context window of models not known to have a larger one.
const defaultContextWindow = 200_000

// largeContextModels are model ids with a 1M context window besides those with the `[1m]` suffix.
var largeContextModels = map[string]bool{}

// ContextWindow is the context window in tokens of a model id.
func ContextWindow(model string) int {
	if strings.HasSuffix(model, "[1m]") || largeContextModels[model] {
		return 1_000_000
	}
	return defaultContextWindow
}

// Tailer reads a Claude session's transcript and its subagent files incrementally. It is not safe for concurrent
// use: one Poll at a time (one tea.Cmd in flight).
type Tailer struct {
	path   string            // the lead's transcript
	dir    string            // <session>/subagents
	lead   *transcript       // the lead's transcript state
	agents map[string]*agent // by agent id
	failed map[string]bool   // tool_use ids whose result is an error
}

type agent struct {
	t       *transcript
	meta    agentMeta
	hasMeta bool
	metaMod time.Time
}

type agentMeta struct {
	AgentType     string `json:"agentType"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	ParentAgentID string `json:"parentAgentId"`
	SpawnDepth    int    `json:"spawnDepth"`
	Model         string `json:"model"`
	ToolUseID     string `json:"toolUseId"`
	Finished      bool   `json:"finished"`
}

// NewTailer tails the transcript at path and the subagents next to it (<session>/subagents for <session>.jsonl).
func NewTailer(path string) *Tailer {
	return &Tailer{
		path:   path,
		dir:    filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents"),
		lead:   newTranscript(path),
		agents: map[string]*agent{},
		failed: map[string]bool{},
	}
}

// Poll reads what was appended since the last call and returns the state so far. Missing files are no error;
// other read errors are returned along with the state read so far.
func (t *Tailer) Poll() (Snapshot, error) {
	var errs []error
	errs = append(errs, t.lead.read(t.failed))
	entries, err := os.ReadDir(t.dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		errs = append(errs, err)
	}
	for _, e := range entries {
		name := e.Name()
		id, ok := strings.CutPrefix(name, "agent-")
		if !ok {
			continue
		}
		switch {
		case strings.HasSuffix(id, ".meta.json"):
			errs = append(errs, t.agent(strings.TrimSuffix(id, ".meta.json")).readMeta(filepath.Join(t.dir, name)))
		case strings.HasSuffix(id, ".jsonl"):
			errs = append(errs, t.agent(strings.TrimSuffix(id, ".jsonl")).t.read(t.failed))
		}
	}
	return t.snapshot(), errors.Join(errs...)
}

func (t *Tailer) agent(id string) *agent {
	a := t.agents[id]
	if a == nil {
		a = &agent{t: newTranscript(filepath.Join(t.dir, "agent-"+id+".jsonl"))}
		t.agents[id] = a
	}
	return a
}

// readMeta re-reads the meta file when it changed: Claude rewrites it (e.g. on finish) instead of appending.
func (a *agent) readMeta(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if a.hasMeta && fi.ModTime().Equal(a.metaMod) {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	var m agentMeta
	if json.Unmarshal(b, &m) != nil {
		return nil // half-written: read it again next time
	}
	a.meta, a.hasMeta, a.metaMod = m, true, fi.ModTime()
	return nil
}

func (t *Tailer) snapshot() Snapshot {
	l := t.lead
	s := Snapshot{Lead: Lead{
		Model:     l.model,
		Tool:      l.tool,
		ToolArg:   l.toolArg,
		Turns:     len(l.msgs),
		Calls:     l.calls,
		ToolCalls: make(map[string]int, len(l.toolCalls)),
		Tokens:    l.tokens,
		LastText:  l.lastText,
	}}
	for k, v := range l.toolCalls {
		s.Lead.ToolCalls[k] = v
	}
	for id, a := range t.agents {
		s.Agents = append(s.Agents, a.snapshot(id, t.failed))
	}
	sort.Slice(s.Agents, func(i, j int) bool {
		a, b := s.Agents[i], s.Agents[j]
		if !a.Start.Equal(b.Start) {
			return a.Start.Before(b.Start)
		}
		return a.ID < b.ID
	})
	return s
}

func (a *agent) snapshot(id string, failed map[string]bool) Agent {
	m, tr := a.meta, a.t
	out := Agent{
		ID:      id,
		Parent:  m.ParentAgentID,
		Depth:   max(m.SpawnDepth, 1),
		Type:    m.AgentType,
		Model:   tr.model,
		Task:    m.Description,
		Start:   tr.first,
		Tool:    tr.tool,
		ToolArg: tr.toolArg,
		Ctx:     -1,
	}
	out.Name = m.Name
	if out.Name == "" {
		out.Name = briefName(tr.prompt)
	}
	if out.Name == "" {
		out.Name = m.AgentType
	}
	if out.Name == "" {
		out.Name = id
	}
	if out.Model == "" {
		out.Model = m.Model
	}
	if tr.ctxTokens > 0 {
		out.Ctx = tr.ctxTokens * 100 / ContextWindow(out.Model)
	}
	switch {
	case m.ToolUseID != "" && failed[m.ToolUseID]:
		out.Status = AgentError
	case m.Finished:
		out.Status = AgentDone
	}
	if out.Status != AgentRunning {
		out.End, out.Tool, out.ToolArg = tr.last, "", ""
	}
	return out
}

var briefRe = regexp.MustCompile(`Read and follow \S*?briefs/([A-Za-z0-9_-]+)\.md`)

// briefName is the role of a harness agent: the basename of the brief its prompt says to follow, or "".
func briefName(prompt string) string {
	if m := briefRe.FindStringSubmatch(prompt); m != nil {
		return m[1]
	}
	return ""
}

// transcript is the state derived from one JSONL transcript, read from offset on.
type transcript struct {
	path      string
	offset    int64
	msgs      map[string]bool // assistant message ids seen: one message spans several lines
	model     string
	tool      string
	toolArg   string
	toolID    string
	calls     int
	toolCalls map[string]int
	tokens    int
	ctxTokens int // input + cache read + cache write of the newest assistant message
	lastText  string
	prompt    string // the first user prompt
	first     time.Time
	last      time.Time
}

func newTranscript(path string) *transcript {
	return &transcript{path: path, msgs: map[string]bool{}, toolCalls: map[string]int{}}
}

// read applies the complete lines appended since the last read; a half-written last line waits for the next one.
func (t *transcript) read(failed map[string]bool) error {
	f, err := os.Open(t.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()
	if _, err := f.Seek(t.offset, io.SeekStart); err != nil {
		return err
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	end := bytes.LastIndexByte(b, '\n')
	if end < 0 {
		return nil
	}
	for _, line := range bytes.Split(b[:end], []byte{'\n'}) {
		t.apply(line, failed)
	}
	t.offset += int64(end + 1)
	return nil
}

type transcriptLine struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	IsMeta    bool      `json:"isMeta"`
	Message   struct {
		ID      string          `json:"id"`
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
		Usage   struct {
			Input      int `json:"input_tokens"`
			CacheWrite int `json:"cache_creation_input_tokens"`
			CacheRead  int `json:"cache_read_input_tokens"`
			Output     int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
}

func (t *transcript) apply(line []byte, failed map[string]bool) {
	var l transcriptLine
	if json.Unmarshal(line, &l) != nil {
		return
	}
	if l.Type != "user" && l.Type != "assistant" {
		return
	}
	if !l.Timestamp.IsZero() {
		if t.first.IsZero() {
			t.first = l.Timestamp
		}
		t.last = l.Timestamp
	}
	var blocks []contentBlock
	if l.Type == "user" {
		var text string
		if json.Unmarshal(l.Message.Content, &text) == nil {
			if t.prompt == "" && !l.IsMeta {
				t.prompt = text
			}
			return
		}
	}
	if json.Unmarshal(l.Message.Content, &blocks) != nil {
		return
	}
	if l.Type == "user" {
		for _, c := range blocks {
			switch {
			case c.Type == "text" && t.prompt == "" && !l.IsMeta:
				t.prompt = c.Text
			case c.Type == "tool_result":
				if c.IsError {
					failed[c.ToolUseID] = true
				}
				if c.ToolUseID == t.toolID {
					t.tool, t.toolArg, t.toolID = "", "", ""
				}
			}
		}
		return
	}
	m := l.Message
	if m.Model == "<synthetic>" { // Claude Code's own error notices, not a model's answer
		return
	}
	if m.Model != "" {
		t.model = m.Model
	}
	if m.ID != "" && !t.msgs[m.ID] {
		t.msgs[m.ID] = true
		u := m.Usage
		t.tokens += u.Input + u.CacheWrite + u.Output
		t.ctxTokens = u.Input + u.CacheRead + u.CacheWrite
	}
	for _, c := range blocks {
		switch c.Type {
		case "text":
			if s := strings.TrimSpace(c.Text); s != "" {
				t.lastText = s
			}
		case "tool_use":
			t.calls++
			t.toolCalls[c.Name]++
			t.tool, t.toolArg, t.toolID = c.Name, toolArg(c.Input), c.ID
		}
	}
}

// toolArgKeys are the input fields that best say what a tool call is about, in order of preference.
var toolArgKeys = []string{"file_path", "pattern", "command", "description", "url", "query", "skill"}

// toolArg is a short argument of a tool call: the first line of its most telling input field, a path's base name.
func toolArg(input json.RawMessage) string {
	var in map[string]any
	if json.Unmarshal(input, &in) != nil {
		return ""
	}
	for _, k := range toolArgKeys {
		s, ok := in[k].(string)
		if !ok || strings.TrimSpace(s) == "" {
			continue
		}
		if k == "file_path" {
			return filepath.Base(s)
		}
		s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
		return s
	}
	return ""
}
