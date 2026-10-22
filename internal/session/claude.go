package session

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
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
// state and writes it for the session named in $LANES_SESSION. Never fails loudly: it must not disturb claude.
func RunHook(event string) {
	name := os.Getenv("LANES_SESSION")
	if name == "" {
		return
	}
	var in struct {
		NotificationType string `json:"notification_type"`
	}
	if b, err := io.ReadAll(os.Stdin); err == nil {
		_ = json.Unmarshal(b, &in)
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
	_ = os.WriteFile(hookStatePath(name), b, 0o644)
}

// SettingsFile writes the Claude settings (hooks only) that actions pass via `claude --settings`.
func SettingsFile() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	hook := func(event string) []map[string]any {
		return []map[string]any{{"hooks": []map[string]any{{"type": "command", "command": "'" + exe + "' hook " + event}}}}
	}
	settings := map[string]any{"hooks": map[string]any{
		"UserPromptSubmit": hook("UserPromptSubmit"),
		"PreToolUse":       hook("PreToolUse"),
		"Stop":             hook("Stop"),
		"Notification":     hook("Notification"),
	}}
	b, _ := json.MarshalIndent(settings, "", "  ")
	p := filepath.Join(app.StateDir(), "claude-settings.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	return p, os.WriteFile(p, b, 0o644)
}
