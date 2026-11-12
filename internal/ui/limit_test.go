package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/history"
	"github.com/chriha/lanes/internal/session"
)

func intp(n int) *int { return &n }

// limitModel is a busyModel whose sessions start through a fake tmux; it returns the tmux call log.
func limitModel(t *testing.T, limit *int, live []session.Info) (*Model, string) {
	t.Helper()
	m, _ := busyModel(t)
	log := fakeTmux(t)
	mgr, err := session.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	m.sess = mgr
	m.cfg.Settings.MaxSessions = limit
	m.sessions = live
	for _, s := range live {
		m.sessByItem[s.ItemKey] = append(m.sessByItem[s.ItemKey], s)
	}
	return m, log
}

// starts counts the sessions the fake tmux was asked to create.
func starts(t *testing.T, log string) int {
	t.Helper()
	raw, _ := os.ReadFile(log)
	return strings.Count(string(raw), "new-session ")
}

// others are live sessions of items that are not listed, so they only count toward the limit.
func others(n int, dead bool) []session.Info {
	var out []session.Info
	for i := range n {
		key := string(rune('a' + i))
		out = append(out, session.Info{Name: "work~" + key, Action: "work", ItemKey: key, Dead: dead})
	}
	return out
}

// Starting a session action asks to confirm when the live action sessions plus the new ones exceed
// max_claude_sessions; "no" starts nothing. Under the limit, or with limit 0, nothing changes.
func TestSessionLimit(t *testing.T) {
	tests := []struct {
		name       string
		limit      *int // nil = unset (3)
		live       []session.Info
		busy       map[string]string // item key -> action holding it
		action     string            // default "review"
		targets    []string          // marked items; empty = the cursor item 1
		answer     string            // key pressed at the prompt
		wantPrompt string            // substring of the prompt; "" = no prompt
		noLimit    bool              // the prompt must not mention the limit
		wantStarts int
	}{
		{name: "under the default limit: no prompt", live: others(2, false), wantStarts: 1},
		{name: "at the default limit: asks, yes starts", live: others(3, false), answer: "y",
			wantPrompt: `Limit of 3 reached (3 live). Run "review" on 1 anyway?`, wantStarts: 1},
		{name: "at the limit: no starts nothing", live: others(3, false), answer: "n",
			wantPrompt: "Limit of 3 reached", wantStarts: 0},
		{name: "configured limit", limit: intp(5), live: others(4, false), wantStarts: 1},
		{name: "configured limit exceeded", limit: intp(5), live: others(5, false), answer: "y",
			wantPrompt: "Limit of 5 reached", wantStarts: 1},
		{name: "limit 0: never asks", limit: intp(0), live: others(10, false), wantStarts: 1},
		{name: "batch over the limit: one prompt naming the limit, yes starts all", live: others(1, false),
			targets: []string{"1", "2", "3"}, answer: "y",
			wantPrompt: `Limit of 3 reached (1 live). Run "review" on 3 items (1, 2, 3) anyway?`, wantStarts: 3},
		{name: "batch over the limit: no starts none", live: others(1, false),
			targets: []string{"1", "2", "3"}, answer: "n", wantPrompt: "Limit of 3 reached", wantStarts: 0},
		{name: "batch within the limit: today's prompt", targets: []string{"1", "2", "3"}, answer: "y",
			wantPrompt: `Run "review" on 3 items (1, 2, 3)?`, noLimit: true, wantStarts: 3},
		{name: "dead sessions are not counted", limit: intp(1), live: others(3, true), wantStarts: 1},
		{name: "starting session actions are counted", limit: intp(2), live: others(1, false),
			busy: map[string]string{"z": "review"}, answer: "y", wantPrompt: "Limit of 2 reached (2 live)", wantStarts: 1},
		{name: "busy headless runs are not counted", limit: intp(2), live: others(1, false),
			busy: map[string]string{"z": "quick"}, wantStarts: 1},
		{name: "restarting a dead session counts as new", limit: intp(1),
			live:   append(others(1, false), session.Info{Name: "review~1", Action: "review", ItemKey: "1", Dead: true}),
			answer: "y", wantPrompt: "Limit of 1 reached (1 live)", wantStarts: 1},
		{name: "headless run actions are not gated", limit: intp(1), live: others(3, false), action: "quick"},
		{name: "already running item: nothing new, no prompt", limit: intp(1),
			live: []session.Info{{Name: "review~1", Action: "review", ItemKey: "1"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, log := limitModel(t, tt.limit, tt.live)
			for key, action := range tt.busy {
				m.busy[key] = busyState{action: action}
			}
			for _, id := range tt.targets {
				m.marks[id] = true
			}
			name := tt.action
			if name == "" {
				name = "review"
			}
			cmd := m.runAction(name, m.cfg.Action[name])
			prompt := ""
			if m.mode == modeConfirm {
				prompt = m.confirm.prompt
				cmd = press(m, tt.answer)
				if m.mode == modeConfirm {
					t.Fatalf("a second prompt followed: %q", m.confirm.prompt)
				}
			}
			settle(m, cmd)

			switch {
			case tt.wantPrompt == "" && prompt != "":
				t.Errorf("prompt = %q, want none", prompt)
			case tt.wantPrompt != "" && !strings.Contains(prompt, tt.wantPrompt):
				t.Errorf("prompt = %q, want it to contain %q", prompt, tt.wantPrompt)
			case tt.noLimit && strings.Contains(prompt, "Limit"):
				t.Errorf("prompt = %q, want no limit in it", prompt)
			}
			if got := starts(t, log); got != tt.wantStarts {
				t.Errorf("started %d session(s), want %d (notice %q)", got, tt.wantStarts, m.notice)
			}
		})
	}
}

// gr / :resume asks the same question over the limit; yes resumes, no starts nothing.
func TestResumeSessionLimit(t *testing.T) {
	tests := []struct {
		name       string
		limit      *int
		live       int
		answer     string
		wantPrompt bool
		wantStarts int
	}{
		{name: "under the limit: resumes", limit: intp(2), live: 1, wantStarts: 1},
		{name: "over the limit: yes resumes", limit: intp(1), live: 1, answer: "y", wantPrompt: true, wantStarts: 1},
		{name: "over the limit: no starts nothing", limit: intp(1), live: 1, answer: "n", wantPrompt: true},
		{name: "limit 0: resumes", limit: intp(0), live: 5, wantStarts: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, log := limitModel(t, tt.limit, others(tt.live, false))
			m.cfg.Action = map[string]config.Action{"fix": {Run: "echo fix", Resume: "claude --resume {{.claude_session}}"}}
			history.Append(history.Event{Item: "1", Action: "fix", Event: "finished", ClaudeSession: "abc"})

			cmd := press(m, "g", "r")
			prompted := m.mode == modeConfirm
			if prompted {
				if !strings.Contains(m.confirm.prompt, "Limit of 1 reached") {
					t.Errorf("prompt = %q, want it to name the limit", m.confirm.prompt)
				}
				cmd = press(m, tt.answer)
			}
			settle(m, cmd)

			if prompted != tt.wantPrompt {
				t.Errorf("prompted = %v, want %v", prompted, tt.wantPrompt)
			}
			if got := starts(t, log); got != tt.wantStarts {
				t.Errorf("started %d session(s), want %d (notice %q)", got, tt.wantStarts, m.notice)
			}
		})
	}
}

// A chain follow-up starts unattended, so the limit never holds it back.
func TestChainIgnoresSessionLimit(t *testing.T) {
	m, log := limitModel(t, intp(1), others(2, false))
	m.cfg.Action = map[string]config.Action{"work": {Run: "echo work", OnSuccess: "review"}, "review": {Run: "echo review"}}

	settle(m, m.chain("work", "1", 0))

	if m.mode == modeConfirm {
		t.Fatalf("chain asked %q, want no prompt", m.confirm.prompt)
	}
	if got := starts(t, log); got != 1 {
		t.Errorf("started %d session(s), want the follow-up (notice %q)", got, m.notice)
	}
}
