package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/history"
	"github.com/chriha/lanes/internal/session"
)

// gr / :resume start the resume template of the action whose Claude session ended last on the cursor item.
func TestResume(t *testing.T) {
	ended := []history.Event{
		{Item: "1", Action: "fix", Event: "finished", ClaudeSession: "old"},
		{Item: "1", Action: "fix", Event: "killed", ClaudeSession: "abc"},
		{Item: "1", Action: "review", Event: "finished", ClaudeSession: "noresume"}, // review has no resume
		{Item: "1", Action: "fix", Event: "started"},
		{Item: "2", Action: "fix", Event: "finished", ClaudeSession: "other"}, // another item
	}
	tests := []struct {
		name   string
		keys   []string // pressed; empty runs :resume
		events []history.Event
		sess   *session.Info // existing session of item 1
		want   []string      // substrings of the tmux calls, in order
		notice string        // substring of the notice, empty = not checked
	}{
		{name: "gr starts the newest resumable session", keys: []string{"g", "r"}, events: ended,
			want: []string{"new-session -d -s fix~1 -x 220 -y 55 -c /work/1 -- /bin/sh -ic export LANES_SESSION='fix~1'"}},
		{name: ":resume starts it too", events: ended,
			want: []string{"new-session -d -s fix~1"}},
		{name: "renders the claude session id", keys: []string{"g", "r"}, events: ended,
			want: []string{"; claude --resume 'abc'"}},
		{name: "dead session: restarts it", keys: []string{"g", "r"}, events: ended,
			sess: &session.Info{Name: "fix~1", Action: "fix", ItemKey: "1", Dead: true},
			want: []string{"kill-session -t =fix~1", "new-session -d -s fix~1"}},
		{name: "running session: notice, nothing starts", keys: []string{"g", "r"}, events: ended,
			sess:   &session.Info{Name: "fix~1", Action: "fix", ItemKey: "1"},
			notice: "work is still running — enter jumps"},
		{name: "nothing resumable: notice", keys: []string{"g", "r"},
			events: []history.Event{{Item: "1", Action: "review", Event: "finished", ClaudeSession: "x"}, {Item: "1", Action: "fix", Event: "finished"}},
			notice: "nothing to resume"},
		{name: "no history: notice", keys: []string{"g", "r"}, notice: "nothing to resume"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := busyModel(t)
			log := fakeTmux(t)
			mgr, err := session.NewManager()
			if err != nil {
				t.Fatal(err)
			}
			m.sess = mgr
			m.cfg = &config.Config{
				Action: map[string]config.Action{
					"fix":    {Run: "echo fix", Resume: "claude --resume {{.claude_session}}", Cwd: "/work/{{.id}}"},
					"review": {Run: "echo review"},
				},
				Keymap: map[string]string{"r": "review"}, // gr must not run it
			}
			if tt.sess != nil {
				m.sessions = []session.Info{*tt.sess}
				m.sessByItem["1"] = m.sessions
			}
			for _, e := range tt.events {
				history.Append(e)
			}

			if len(tt.keys) > 0 {
				settle(m, press(m, tt.keys...))
			} else {
				_, cmd := m.runCommand("resume")
				settle(m, cmd)
			}

			raw, _ := os.ReadFile(log)
			calls := strings.Split(strings.TrimSpace(string(raw)), "\n")
			if len(raw) == 0 {
				calls = nil
			}
			if len(calls) != len(tt.want) {
				t.Fatalf("tmux calls:\n%s\nwant %d matching %q (notice %q)", raw, len(tt.want), tt.want, m.notice)
			}
			for i, w := range tt.want {
				if !strings.Contains(calls[i], w) {
					t.Errorf("call %d = %q, want it to contain %q", i, calls[i], w)
				}
			}
			if tt.notice != "" && !strings.Contains(m.notice, tt.notice) {
				t.Errorf("notice = %q, want it to contain %q", m.notice, tt.notice)
			}
		})
	}
}

// gr resumes the cursor item only: marks on other items stay and are not resumed.
func TestResumeKeepsMarks(t *testing.T) {
	m, _ := busyModel(t)
	log := fakeTmux(t)
	mgr, err := session.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	m.sess = mgr
	m.cfg.Action = map[string]config.Action{"fix": {Run: "echo fix", Resume: "claude --resume {{.claude_session}}"}}
	history.Append(history.Event{Item: "1", Action: "fix", Event: "finished", ClaudeSession: "abc"})
	history.Append(history.Event{Item: "2", Action: "fix", Event: "finished", ClaudeSession: "def"})
	m.marks["2"] = true

	settle(m, press(m, "g", "r"))

	raw, _ := os.ReadFile(log)
	if !strings.Contains(string(raw), "new-session -d -s fix~1") || strings.Contains(string(raw), "fix~2") {
		t.Errorf("tmux calls:\n%s\nwant only fix~1 started", raw)
	}
	if len(m.marks) != 1 || !m.marks["2"] {
		t.Errorf("marks = %v, want item 2 still marked", m.marks)
	}
}
