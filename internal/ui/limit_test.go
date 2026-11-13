package ui

import (
	"os"
	"reflect"
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

// queuedIDs lists the items in the queue, oldest first.
func queuedIDs(m *Model) []string {
	var out []string
	for _, e := range m.auto.queue {
		out = append(out, e.id)
	}
	return out
}

// Starting a session action that would take the live action sessions above max_claude_sessions asks nothing: the
// items that fit start, the rest are queued in item order. Under the limit, or with limit 0, nothing changes; the
// confirm prompt of a batch still shows.
func TestSessionLimit(t *testing.T) {
	tests := []struct {
		name       string
		limit      *int // nil = unset (3)
		live       []session.Info
		busy       map[string]string // item key -> action holding it
		action     string            // default "review"
		targets    []string          // marked items; empty = the cursor item 1
		twice      bool              // start the action a second time
		answer     string            // key pressed at a prompt
		wantPrompt string            // substring of the prompt; "" = no prompt
		wantStarts int
		wantQueued []string
		wantNotice string // substring of the final notice
	}{
		{name: "under the default limit: starts", live: others(2, false), wantStarts: 1},
		{name: "at the default limit: queued, no prompt", live: others(3, false), wantQueued: []string{"1"},
			wantNotice: "queued review on 1 (Limit of 3 reached (3 live))"},
		{name: "configured limit", limit: intp(5), live: others(4, false), wantStarts: 1},
		{name: "configured limit exceeded: queued", limit: intp(5), live: others(5, false), wantQueued: []string{"1"},
			wantNotice: "Limit of 5 reached"},
		{name: "limit 0: never queues", limit: intp(0), live: others(10, false), wantStarts: 1},
		{name: "batch over the limit: what fits starts, the rest queues in item order", live: others(0, false),
			limit: intp(1), targets: []string{"1", "2", "3"}, answer: "y",
			wantPrompt: `Run "review" on 3 items (1, 2, 3)?`, wantStarts: 1, wantQueued: []string{"2", "3"},
			wantNotice: "2 queued (Limit of 1 reached (0 live))"},
		{name: "batch over the limit: no starts and queues nothing", live: others(1, false),
			targets: []string{"1", "2", "3"}, answer: "n", wantPrompt: `Run "review" on 3 items`},
		{name: "batch within the limit: today's prompt", targets: []string{"1", "2", "3"}, answer: "y",
			wantPrompt: `Run "review" on 3 items (1, 2, 3)?`, wantStarts: 3},
		{name: "dead sessions are not counted", limit: intp(1), live: others(3, true), wantStarts: 1},
		{name: "starting session actions are counted", limit: intp(2), live: others(1, false),
			busy: map[string]string{"z": "review"}, wantQueued: []string{"1"}, wantNotice: "Limit of 2 reached (2 live)"},
		{name: "busy headless runs are not counted", limit: intp(2), live: others(1, false),
			busy: map[string]string{"z": "quick"}, wantStarts: 1},
		{name: "restarting a dead session counts as new", limit: intp(1),
			live:       append(others(1, false), session.Info{Name: "review~1", Action: "review", ItemKey: "1", Dead: true}),
			wantQueued: []string{"1"}, wantNotice: "Limit of 1 reached (1 live)"},
		{name: "headless run actions are not gated", limit: intp(1), live: others(3, false), action: "quick"},
		{name: "already running item: nothing new, nothing queued", limit: intp(1),
			live: []session.Info{{Name: "review~1", Action: "review", ItemKey: "1"}}},
		{name: "an item already queued is not queued again", live: others(3, false), twice: true,
			wantQueued: []string{"1"}, wantNotice: "1 already queued"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, log := limitModel(t, tt.limit, tt.live)
			for key, action := range tt.busy {
				m.busy[key] = busyState{action: action}
			}
			name := tt.action
			if name == "" {
				name = "review"
			}
			prompt := ""
			runs := 1
			if tt.twice {
				runs = 2
			}
			for range runs {
				for _, id := range tt.targets {
					m.marks[id] = true
				}
				cmd := m.runAction(name, m.cfg.Action[name])
				if m.mode == modeConfirm {
					prompt = m.confirm.prompt
					cmd = press(m, tt.answer)
					if m.mode == modeConfirm {
						t.Fatalf("a second prompt followed: %q", m.confirm.prompt)
					}
				}
				settle(m, cmd)
			}

			switch {
			case tt.wantPrompt == "" && prompt != "":
				t.Errorf("prompt = %q, want none", prompt)
			case tt.wantPrompt != "" && !strings.Contains(prompt, tt.wantPrompt):
				t.Errorf("prompt = %q, want it to contain %q", prompt, tt.wantPrompt)
			case strings.Contains(prompt, "anyway"):
				t.Errorf("prompt = %q, want no limit question", prompt)
			}
			if got := starts(t, log); got != tt.wantStarts {
				t.Errorf("started %d session(s), want %d (notice %q)", got, tt.wantStarts, m.notice)
			}
			if got := queuedIDs(m); !reflect.DeepEqual(got, tt.wantQueued) {
				t.Errorf("queued %v, want %v", got, tt.wantQueued)
			}
			if !strings.Contains(m.notice, tt.wantNotice) {
				t.Errorf("notice = %q, want it to contain %q", m.notice, tt.wantNotice)
			}
		})
	}
}

// gr / :resume over the limit queues the resume instead of asking; it starts once a poll shows room.
func TestResumeSessionLimit(t *testing.T) {
	tests := []struct {
		name       string
		limit      *int
		live       int
		twice      bool // press gr a second time
		freed      bool // a poll then shows no live sessions
		wantStarts int
		wantQueued []string
		wantNotice string
	}{
		{name: "under the limit: resumes", limit: intp(2), live: 1, wantStarts: 1},
		{name: "over the limit: queued, no prompt", limit: intp(1), live: 1, wantQueued: []string{"1"},
			wantNotice: "queued resume of fix on 1 (Limit of 1 reached (1 live))"},
		{name: "a queued resume starts once there is room", limit: intp(1), live: 1, freed: true, wantStarts: 1,
			wantNotice: "resumed queued fix on 1"},
		{name: "a queued item is not queued again", limit: intp(1), live: 1, twice: true, wantQueued: []string{"1"},
			wantNotice: "1 already queued"},
		{name: "limit 0: resumes", limit: intp(0), live: 5, wantStarts: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, log := limitModel(t, tt.limit, others(tt.live, false))
			m.cfg.Action = map[string]config.Action{"fix": {Run: "echo fix", Resume: "claude --resume {{.claude_session}}"}}
			history.Append(history.Event{Item: "1", Action: "fix", Event: "finished", ClaudeSession: "abc"})

			settle(m, press(m, "g", "r"))
			if tt.twice {
				settle(m, press(m, "g", "r"))
			}
			if m.mode == modeConfirm {
				t.Fatalf("resume asked %q, want no prompt", m.confirm.prompt)
			}
			if tt.freed {
				poll()(t, m, nil)
			}

			if got := starts(t, log); got != tt.wantStarts {
				t.Errorf("started %d session(s), want %d (notice %q)", got, tt.wantStarts, m.notice)
			}
			if got := queuedIDs(m); !reflect.DeepEqual(got, tt.wantQueued) {
				t.Errorf("queued %v, want %v", got, tt.wantQueued)
			}
			if !strings.Contains(m.notice, tt.wantNotice) {
				t.Errorf("notice = %q, want it to contain %q", m.notice, tt.wantNotice)
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

// The built-in shell is a plain user shell: over max_claude_sessions it starts right away (and jumps) instead of
// queueing a start that would later run without a jump.
func TestShellIgnoresSessionLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit *int
		live  []session.Info
	}{
		{name: "at the default limit", live: others(3, false)},
		{name: "over a configured limit", limit: intp(1), live: others(4, false)},
		{name: "under the limit", live: others(1, false)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, log := limitModel(t, tt.limit, tt.live)
			m.marks["2"] = true // ignored by the shell

			settle(m, m.shell(m.items()[0]))

			if got := starts(t, log); got != 1 {
				t.Errorf("started %d session(s), want 1 (notice %q)", got, m.notice)
			}
			if got := queuedIDs(m); len(got) != 0 {
				t.Errorf("queued %v, want nothing", got)
			}
			if !m.marks["2"] {
				t.Errorf("marks = %v, want them kept", m.marks)
			}
		})
	}
}
