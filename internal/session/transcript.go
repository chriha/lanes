package session

import "time"

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

// Tailer reads a Claude session's transcript and its subagent files incrementally.
type Tailer struct{}

// NewTailer tails the transcript at path and the subagents next to it.
func NewTailer(path string) *Tailer { return &Tailer{} }

// Poll reads what was appended since the last call and returns the state so far.
func (t *Tailer) Poll() (Snapshot, error) { return Snapshot{}, nil }

// ContextWindow is the context window in tokens of a model id.
func ContextWindow(model string) int { return 0 }
