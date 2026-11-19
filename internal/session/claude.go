package session

import (
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chriha/lanes/internal/app"
)

func statePath(name string) string {
	return filepath.Join(app.StateDir(), "sessions", name+".json")
}

// hookStatePath is used inside the session, where the project scope is not set: the dir comes from the env.
func hookStatePath(name string) string {
	return filepath.Join(os.Getenv("LANES_STATE_DIR"), "sessions", name+".json")
}

// claudePath is the sidecar where the hooks keep the ids of the newest Claude session seen in session name.
func claudePath(name string) string {
	return filepath.Join(app.StateDir(), "sessions", name+".claude.json")
}

func hookClaudePath(name string) string {
	return filepath.Join(os.Getenv("LANES_STATE_DIR"), "sessions", name+".claude.json")
}

// InSession tells whether the process runs in a session lanes started: Start exports both variables.
func InSession() bool { return os.Getenv("LANES_SESSION") != "" && os.Getenv("LANES_STATE_DIR") != "" }

// claudeIDs are the ids a Claude hook payload carries and the sidecar keeps.
type claudeIDs struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
}

// readClaudeIDs reads the sidecar of session name; a missing or corrupt one gives empty ids.
func readClaudeIDs(name string) claudeIDs {
	var ids claudeIDs
	b, err := os.ReadFile(claudePath(name))
	if err != nil || json.Unmarshal(b, &ids) != nil {
		return claudeIDs{}
	}
	return ids
}

// ClaudeSession returns the id of the newest Claude session the hooks saw in the session, or "".
func ClaudeSession(name string) string { return readClaudeIDs(name).SessionID }

// ClaudeTranscript returns the transcript path of the newest Claude session the hooks saw in the session, or "".
func ClaudeTranscript(name string) string { return readClaudeIDs(name).TranscriptPath }

// ForgetClaude removes the Claude session ids the hooks recorded for the session.
func ForgetClaude(name string) { _ = os.Remove(claudePath(name)) }

// hookState is the content of a session's state file. Background marks a launched background job (Bash or
// subagent) that Claude has not been re-invoked for since; older lanes ignore the field.
type hookState struct {
	State      string `json:"state"`
	At         int64  `json:"at"`
	Background bool   `json:"background,omitempty"`
}

func readState(name string) string {
	b, err := os.ReadFile(statePath(name))
	if err != nil {
		return ""
	}
	var s hookState
	_ = json.Unmarshal(b, &s)
	return s.State
}

// readHookState reads the state file from inside the session; a missing or corrupt one gives the zero value.
func readHookState(name string) hookState {
	var s hookState
	if b, err := os.ReadFile(hookStatePath(name)); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

// RunHook is the `<app> hook <event>` entry point called by Claude Code hooks. It maps the event to a
// state and writes it (SessionEnd removes it) for the session named in $LANES_SESSION. Every event also keeps the
// payload's Claude session id in a sidecar (SessionEnd too), for lanes to record with the session's end. Never
// fails loudly: it must not disturb claude.
func RunHook(event string) {
	if !InSession() { // an empty state dir would make the state paths relative to cwd
		return
	}
	name := os.Getenv("LANES_SESSION")
	var in struct {
		claudeIDs
		NotificationType string `json:"notification_type"`
		Reason           string `json:"reason"`
		ToolInput        struct {
			RunInBackground bool `json:"run_in_background"`
		} `json:"tool_input"`
	}
	if b, err := io.ReadAll(os.Stdin); err == nil {
		_ = json.Unmarshal(b, &in)
	}
	if in.SessionID != "" {
		b, _ := json.Marshal(in.claudeIDs)
		_ = app.WriteFile(hookClaudePath(name), b)
	}
	if event == "SessionEnd" {
		// /clear and resume end a session but Claude keeps running: keep the state then.
		if in.Reason != "clear" && in.Reason != "resume" {
			_ = os.Remove(hookStatePath(name))
		}
		return
	}
	state := map[string]string{
		"UserPromptSubmit": "working",
		"PreToolUse":       "working",
		"Stop":             "waiting",
		"Notification":     "asking",
		"SubagentStop":     "", // only clears the marker; the state stays
	}[event]
	// A launch sets the marker; a prompt or a finished subagent clears it. Other tool calls leave it: the job outlives them.
	prev := readHookState(name)
	background := prev.Background
	switch event {
	case "PreToolUse":
		background = background || in.ToolInput.RunInBackground
	case "UserPromptSubmit", "SubagentStop":
		background = false
	}
	idle := event == "Notification" && in.NotificationType == "idle_prompt"
	if idle {
		state = "waiting"
	}
	if background && (event == "Stop" || idle) {
		state = "background"
	}
	if event == "SubagentStop" {
		state = prev.State // keep the state, drop the marker
		if state == "background" {
			state = "waiting" // Claude stays idle, but no job is pending any more
		}
	}
	if state == "" {
		return
	}
	b, _ := json.Marshal(hookState{State: state, At: time.Now().Unix(), Background: background})
	_ = app.WriteFile(hookStatePath(name), b)
}

// hookCommand is the shell command Claude runs for a hook event.
func hookCommand(exe, event string) string { return shq(exe) + " hook " + event }

// SettingsFile writes the Claude settings (hooks only) that actions pass via `claude --settings`.
func SettingsFile() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	hook := func(event string) []map[string]any {
		return []map[string]any{{"hooks": []map[string]any{{"type": "command", "command": hookCommand(exe, event)}}}}
	}
	settings := map[string]any{"hooks": map[string]any{
		"UserPromptSubmit": hook("UserPromptSubmit"),
		"PreToolUse":       hook("PreToolUse"),
		"Stop":             hook("Stop"),
		"SubagentStop":     hook("SubagentStop"),
		"Notification":     hook("Notification"),
		"SessionEnd":       hook("SessionEnd"),
	}}
	b, _ := json.MarshalIndent(settings, "", "  ")
	p := filepath.Join(app.StateDir(), "claude-settings.json")
	if err := os.MkdirAll(filepath.Dir(p), app.DirMode); err != nil {
		return "", err
	}
	return p, app.WriteFile(p, b)
}

// UsageFile is what a Claude Code mod writes into a Claude config dir: the statusline's usage text and when.
const UsageFile = "usage-line.json"

// usageLine is what Usage and Usage5h read of the usage file.
type usageLine struct {
	Text    string          `json:"text"`
	Session json.RawMessage `json:"session"` // the account's 5h usage in percent; raw, so a bad one spares Text
	At      int64           `json:"at"`
}

// usageEntry is a parsed usage file with the mtime and size it had when read.
type usageEntry struct {
	mtime time.Time
	size  int64
	line  usageLine
	ok    bool
}

var (
	usageMu    sync.Mutex
	usageCache = map[string]usageEntry{} // by usage file path
)

// readUsage reads the usage file in the Claude config dir (a leading ~ is the home dir). ok is false when dir is
// empty or the file is missing or unreadable. The parsed file is cached by path, mtime and size, so an unchanged
// file is read once however often readUsage is called.
func readUsage(dir string) (u usageLine, ok bool) {
	if dir == "" {
		return u, false
	}
	if rest, ok := strings.CutPrefix(dir, "~"); ok {
		home, _ := os.UserHomeDir()
		dir = home + rest
	}
	p := filepath.Join(dir, UsageFile)
	usageMu.Lock()
	defer usageMu.Unlock()
	fi, err := os.Stat(p)
	if err != nil {
		delete(usageCache, p)
		return usageLine{}, false
	}
	if e, hit := usageCache[p]; hit && e.mtime.Equal(fi.ModTime()) && e.size == fi.Size() {
		return e.line, e.ok
	}
	e := usageEntry{mtime: fi.ModTime(), size: fi.Size()}
	if b, err := os.ReadFile(p); err == nil && json.Unmarshal(b, &e.line) == nil {
		e.ok = true
	} else {
		e.line = usageLine{}
	}
	usageCache[p] = e
	return e.line, e.ok
}

// Usage reads the usage text written into the Claude config dir (a leading ~ is the home dir). It returns ""
// when dir is empty or the file is missing or unreadable.
func Usage(dir string) (string, time.Time) {
	u, ok := readUsage(dir)
	if !ok || u.Text == "" {
		return "", time.Time{}
	}
	return u.Text, time.Unix(u.At, 0)
}

// UsageStale is the age after which Usage5h no longer trusts the usage file.
const UsageStale = 15 * time.Minute

// Usage5h reads the account's 5h usage in percent (the `session` value) from the usage file in the Claude config
// dir. ok is false when dir is empty, the file is missing or unreadable, has no `session` value, or was written
// more than UsageStale before now.
func Usage5h(dir string, now time.Time) (pct int, ok bool) {
	u, ok := readUsage(dir)
	var p *float64
	if !ok || json.Unmarshal(u.Session, &p) != nil || p == nil || u.At == 0 || now.Sub(time.Unix(u.At, 0)) > UsageStale {
		return 0, false
	}
	return int(math.Round(*p)), true
}
