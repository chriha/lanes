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
	"slices"
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

	Effort       string // effort level of the newest assistant message that has one ("low" … "max"), "" when unknown
	Ctx          int    // percent of the context window used by the newest assistant message, -1 when unknown
	TokensIn     int    // input tokens of all assistant messages
	TokensOut    int    // output tokens of all assistant messages
	TokensCache  int    // cache read + cache write tokens of all assistant messages
	AdvisorModel string // the advisor model of the newest assistant message that names one, "" when none
	AdvisorCalls int    // advisor calls in total
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
	Ctx     int    // percent of the context window used by the newest assistant message, -1 when unknown
	Effort  string // effort level of the newest assistant message that has one, "" when unknown
}

// Snapshot is the state of a Claude session derived from its transcripts.
type Snapshot struct {
	Lead   Lead
	Agents []Agent // ordered by start, then id

	// ToolTimes are the start times of the tool calls of the lead and all agents within ToolWindow of the newest
	// transcript line, oldest first. The view windows them again against its own clock.
	ToolTimes []time.Time
	// Files are the base names of the last MaxFiles files Edit, Write or NotebookEdit touched (lead and agents),
	// newest first, each once.
	Files []string
}

const (
	ToolWindow = 15 * time.Minute // how far back Snapshot.ToolTimes reaches
	MaxFiles   = 4                // how many files Snapshot.Files holds at most
)

// defaultContextWindow is the context window of models not known to have a larger one.
const defaultContextWindow = 200_000

// largeContextModels are model ids with a 1M context window besides those with the `[1m]` suffix.
var largeContextModels = map[string]bool{}

// ContextWindow is the context window in tokens of a model id. Transcripts don't always carry the `[1m]` suffix
// of a 1M session: ctxPercent also takes the larger window when the tokens don't fit the smaller one.
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
	metaLen int64
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
	if a.hasMeta && (a.meta.Finished || fi.ModTime().Equal(a.metaMod) && fi.Size() == a.metaLen) {
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
	a.meta, a.hasMeta, a.metaMod, a.metaLen = m, true, fi.ModTime(), fi.Size()
	return nil
}

func (t *Tailer) snapshot() Snapshot {
	l := t.lead
	s := Snapshot{Lead: Lead{
		Model:     l.model,
		Tool:      l.current().name,
		ToolArg:   l.current().arg,
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
		Tool:    tr.current().name,
		ToolArg: tr.current().arg,
		Ctx:     -1,
	}
	out.Name = m.Name
	if out.Name == "" {
		out.Name = tr.brief
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
		out.Ctx = ctxPercent(tr.ctxTokens, out.Model)
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

// ctxPercent is the share of the model's context window the tokens use.
func ctxPercent(tokens int, model string) int {
	w := ContextWindow(model)
	if tokens > w {
		w = max(1_000_000, tokens)
	}
	return tokens * 100 / w
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
	msgs      map[string]int // tokens counted per assistant message id: one message spans several lines
	model     string
	pending   []toolCall // tool calls without a result yet, oldest first
	calls     int
	toolCalls map[string]int
	tokens    int
	ctxTokens int // input + cache read + cache write of the newest assistant message
	lastText  string
	brief     string // the brief named in the first user text that names one
	first     time.Time
	last      time.Time
}

func newTranscript(path string) *transcript {
	return &transcript{path: path, msgs: map[string]int{}, toolCalls: map[string]int{}}
}

type toolCall struct{ id, name, arg string }

// current is the newest tool call still running.
func (t *transcript) current() toolCall {
	if len(t.pending) == 0 {
		return toolCall{}
	}
	return t.pending[len(t.pending)-1]
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
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if fi.Size() < t.offset { // rewritten from scratch: start over
		*t = *newTranscript(t.path)
	}
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

// noteBrief keeps the brief the first user text naming one says to follow; injected texts name none.
func (t *transcript) noteBrief(text string) {
	if t.brief == "" {
		t.brief = briefName(text)
	}
}

type transcriptLine struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
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
			t.noteBrief(text)
			return
		}
	}
	if len(l.Message.Content) > 0 && json.Unmarshal(l.Message.Content, &blocks) != nil {
		return
	}
	if l.Type == "user" {
		for _, c := range blocks {
			switch c.Type {
			case "text":
				t.noteBrief(c.Text)
			case "tool_result":
				if c.IsError {
					failed[c.ToolUseID] = true
				}
				t.pending = slices.DeleteFunc(t.pending, func(p toolCall) bool { return p.id == c.ToolUseID })
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
	if m.ID != "" {
		// The newest line of a message has its newest usage: it replaces what earlier lines counted.
		u := m.Usage
		n := u.Input + u.CacheWrite + u.Output
		t.tokens += n - t.msgs[m.ID]
		t.msgs[m.ID] = n
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
			t.pending = append(t.pending, toolCall{c.ID, c.Name, toolArg(c.Input)})
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
