// Package history keeps a per-item log of actions in one JSON-lines file.
package history

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/chriha/lanes/internal/app"
)

type Event struct {
	Time   time.Time `json:"time"`
	Item   string    `json:"item"`
	Action string    `json:"action"`
	Event  string    `json:"event"` // started | finished | failed | killed
	Exit   int       `json:"exit,omitempty"`
	// ClaudeSession is the Claude Code session id of a session's end event, when Claude ran in it.
	ClaudeSession string `json:"claude_session,omitempty"`
}

func path() string { return filepath.Join(app.StateDir(), "history.jsonl") }

func Append(e Event) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	if os.MkdirAll(filepath.Dir(path()), app.DirMode) != nil {
		return
	}
	f, err := os.OpenFile(path(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, app.FileMode)
	if err != nil {
		return
	}
	defer f.Close()
	_ = f.Chmod(app.FileMode) // tighten a file an older version created 0644
	b, _ := json.Marshal(e)
	_, _ = f.Write(append(b, '\n'))
}

// For returns the item's events, oldest first.
func For(itemID string) []Event {
	f, err := os.Open(path())
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []Event
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e Event
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.Item == itemID {
			out = append(out, e)
		}
	}
	return out
}
