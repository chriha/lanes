package session

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
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

// ClaudeSession returns the id of the newest Claude session the hooks saw in the session, or "".
func ClaudeSession(name string) string { return "" }

// ForgetClaude removes the Claude session ids the hooks recorded for the session.
func ForgetClaude(name string) {}

func readState(name string) string {
	b, err := os.ReadFile(statePath(name))
	if err != nil {
		return ""
	}
	var s struct{ State string }
	_ = json.Unmarshal(b, &s)
	return s.State
}

// RunHook is the `<app> hook <event>` entry point called by Claude Code hooks. It maps the event to a
// state and writes it (SessionEnd removes it) for the session named in $LANES_SESSION. Never fails loudly: it must not disturb claude.
func RunHook(event string) {
	name := os.Getenv("LANES_SESSION")
	if name == "" {
		return
	}
	var in struct {
		NotificationType string `json:"notification_type"`
		Reason           string `json:"reason"`
	}
	if b, err := io.ReadAll(os.Stdin); err == nil {
		_ = json.Unmarshal(b, &in)
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
	}[event]
	if event == "Notification" && in.NotificationType == "idle_prompt" {
		state = "waiting"
	}
	if state == "" {
		return
	}
	b, _ := json.Marshal(map[string]any{"state": state, "at": time.Now().Unix()})
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

// Usage reads the usage text written into the Claude config dir (a leading ~ is the home dir). It returns ""
// when dir is empty or the file is missing or unreadable.
func Usage(dir string) (string, time.Time) {
	if dir == "" {
		return "", time.Time{}
	}
	if rest, ok := strings.CutPrefix(dir, "~"); ok {
		home, _ := os.UserHomeDir()
		dir = home + rest
	}
	b, err := os.ReadFile(filepath.Join(dir, UsageFile))
	if err != nil {
		return "", time.Time{}
	}
	var u struct {
		Text string `json:"text"`
		At   int64  `json:"at"`
	}
	if json.Unmarshal(b, &u) != nil || u.Text == "" {
		return "", time.Time{}
	}
	return u.Text, time.Unix(u.At, 0)
}
