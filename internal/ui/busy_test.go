package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/filter"
	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/provider"
	"github.com/chriha/lanes/internal/session"
)

// fakeProv counts its fetches.
type fakeProv struct{ calls int }

func (f *fakeProv) Fetch(context.Context) ([]item.Item, error) {
	f.calls++
	return mkItems("1", "2", "3"), nil
}

var busyActions = map[string]config.Action{
	"review": {Run: "echo review"},                          // session
	"quick":  {Type: "run", Run: "true"},                    // headless
	"ready":  {Type: "run", Run: "true", Refresh: true},     // headless, refetches
	"broken": {Type: "run", Run: "exit 3", Refresh: true},   // headless, fails
	"ask":    {Type: "run", Run: "true", Confirm: "single"}, // headless, confirms on one item
}

// busyModel lists items 1-3 from a fake provider; sessions are never polled and tmux is never run.
func busyModel(t *testing.T) (*Model, *fakeProv) {
	t.Helper()
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "HOME"} {
		t.Setenv(k, t.TempDir())
	}
	t.Setenv("SHELL", "/bin/sh")
	f, err := filter.Compile("true")
	if err != nil {
		t.Fatal(err)
	}
	fp := &fakeProv{}
	cfg := &config.Config{Action: busyActions}
	cfg.Settings.NoNotify = true
	m := &Model{
		cfg:          cfg,
		providers:    map[string]provider.Provider{"p": fp},
		data:         map[string][]item.Item{"p": mkItems("1", "2", "3")},
		fetchedAt:    map[string]time.Time{},
		loading:      map[string]bool{},
		provErr:      map[string]error{},
		fails:        map[string]int{},
		retryAt:      map[string]time.Time{},
		refetchAfter: map[string]bool{},
		busy:         map[string]busyState{},
		marks:        map[string]bool{},
		sess:         &session.Manager{},
		sessByItem:   map[string][]session.Info{},
		polling:      true, // keeps pollSessions from contacting tmux
		views:        []*view{{cfg: config.View{Name: "v", Provider: "p"}, filter: f}},
		width:        120,
		height:       20,
	}
	m.rebuild()
	return m, fp
}

// runAll runs cmd and the commands of any batch it returns.
func runAll(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, runAll(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// finish runs a headless action's command and feeds its doneMsg back to the model.
func finish(t *testing.T, m *Model, cmd tea.Cmd) doneMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command returned")
	}
	d, ok := cmd().(doneMsg)
	if !ok {
		t.Fatal("command did not return a doneMsg")
	}
	m.Update(d)
	return d
}

func TestActionShowsBusyAtOnce(t *testing.T) {
	tests := []struct {
		name   string
		action string
	}{
		{"session action", "review"},
		{"headless run", "quick"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := busyModel(t)
			cmd := m.runAction(tt.action, busyActions[tt.action])
			if cmd == nil {
				t.Fatal("no command returned")
			}
			if got := rowOf(t, m, "1"); !strings.Contains(got, "⋯ "+tt.action) {
				t.Errorf("row %q has no busy badge", got)
			}
			if want := "starting " + tt.action + " on 1…"; m.notice != want {
				t.Errorf("notice = %q, want %q", m.notice, want)
			}
			if tt.action == "review" {
				m.Update(doneMsg{notice: "started review on 1 item(s)", keys: []string{"1"}, ok: true})
			} else {
				finish(t, m, cmd)
			}
			if got := rowOf(t, m, "1"); strings.Contains(got, "⋯") {
				t.Errorf("row %q still busy after doneMsg", got)
			}
		})
	}
}

func TestBusyItemsAreSkipped(t *testing.T) {
	tests := []struct {
		name       string
		busy       map[string]string // item -> action holding it
		marked     []string
		action     string
		wantCmd    bool
		wantNotice []string // substrings of the notice (after the run, when one ran)
		wantMarked []string // marks left after the run
	}{
		{"same action", map[string]string{"1": "quick"}, nil, "quick", false, []string{"1 busy (quick)"}, nil},
		{"other action", map[string]string{"1": "review"}, nil, "quick", false, []string{"1 busy (review)"}, nil},
		{"mixed selection runs the rest", map[string]string{"1": "review"}, []string{"1", "2"}, "quick", true,
			[]string{"done on 1 item(s)", ", 1 busy"}, []string{"1"}},
		{"all busy runs nothing", map[string]string{"1": "review", "2": "ready"}, []string{"1", "2"}, "quick", false,
			[]string{"1 busy (review)", "2 busy (ready)"}, []string{"1", "2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := busyModel(t)
			for id, a := range tt.busy {
				m.busy[session.Key(id)] = busyState{action: a}
			}
			for _, id := range tt.marked {
				m.marks[id] = true
			}
			cmd := m.runAction(tt.action, busyActions[tt.action])
			if m.mode == modeConfirm {
				cmd = press(m, "y")
			}
			if (cmd != nil) != tt.wantCmd {
				t.Fatalf("cmd returned = %v, want %v (notice %q)", cmd != nil, tt.wantCmd, m.notice)
			}
			if cmd != nil {
				finish(t, m, cmd)
			}
			for _, w := range tt.wantNotice {
				if !strings.Contains(m.notice, w) {
					t.Errorf("notice %q does not contain %q", m.notice, w)
				}
			}
			for id, a := range tt.busy {
				if got := m.busy[session.Key(id)].action; got != a {
					t.Errorf("item %s busy with %q, want %q kept", id, got, a)
				}
			}
			if len(m.marks) != len(tt.wantMarked) {
				t.Errorf("marks = %v, want %v", m.marks, tt.wantMarked)
			}
			for _, id := range tt.wantMarked {
				if !m.marks[id] {
					t.Errorf("item %s lost its mark, marks = %v", id, m.marks)
				}
			}
		})
	}
}

func TestConfirmLocksOnlyOnceConfirmed(t *testing.T) {
	tests := []struct {
		name     string
		marked   []string
		action   string
		answer   string
		wantBusy []string
	}{
		{"multi-item, prompt open", []string{"1", "2"}, "review", "", nil},
		{"multi-item, cancelled", []string{"1", "2"}, "review", "n", nil},
		{"multi-item, confirmed", []string{"1", "2"}, "review", "y", []string{"1", "2"}},
		{"confirm single, prompt open", nil, "ask", "", nil},
		{"confirm single, confirmed", nil, "ask", "y", []string{"1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := busyModel(t)
			for _, id := range tt.marked {
				m.marks[id] = true
			}
			if cmd := m.runAction(tt.action, busyActions[tt.action]); cmd != nil || m.mode != modeConfirm {
				t.Fatalf("no confirmation prompt (mode %d)", m.mode)
			}
			if tt.answer != "" {
				press(m, tt.answer)
			}
			if len(m.busy) != len(tt.wantBusy) {
				t.Errorf("busy = %v, want %v", m.busy, tt.wantBusy)
			}
			for _, id := range tt.wantBusy {
				if m.busy[session.Key(id)].action != tt.action {
					t.Errorf("item %s not busy with %s: %v", id, tt.action, m.busy)
				}
			}
		})
	}
}

func TestRefreshRunHoldsLockUntilRefetch(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name      string
		inFlight  bool       // a fetch started before the run is still loading at doneMsg
		landing   fetchedMsg // what the post-run fetch delivers (items filled in when nil and no error)
		wantFirst int        // fetches issued by doneMsg
		wantClear bool       // lock cleared after landing
	}{
		{"idle provider refetches at once", false, fetchedMsg{name: "p"}, 1, true},
		{"in-flight fetch gets a follow-up", true, fetchedMsg{name: "p"}, 0, true},
		{"error counts as finished", false, fetchedMsg{name: "p", err: boom}, 1, true},
		{"partial result does not count", false, fetchedMsg{name: "p", partial: true}, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, fp := busyModel(t)
			m.loading["p"] = tt.inFlight
			cmd := m.runAction("ready", busyActions["ready"])
			d, ok := cmd().(doneMsg)
			if !ok || !d.ok || !d.refresh {
				t.Fatalf("run result %+v", d)
			}
			_, next := m.Update(d)
			runAll(next)
			if fp.calls != tt.wantFirst {
				t.Fatalf("fetches after doneMsg = %d, want %d", fp.calls, tt.wantFirst)
			}
			if !strings.Contains(rowOf(t, m, "1"), "⋯ ready") || len(m.busy[session.Key("1")].refetch) == 0 {
				t.Fatalf("item not held after a successful refresh run: %v", m.busy)
			}
			if tt.inFlight {
				// the pre-run fetch lands: stale, so the lock holds and a follow-up fetch is issued
				_, next = m.Update(fetchedMsg{name: "p", items: mkItems("1", "2", "3")})
				if _, held := m.busy[session.Key("1")]; !held {
					t.Fatal("lock cleared by a fetch that started before the run")
				}
				runAll(next)
				if fp.calls != 1 {
					t.Fatalf("follow-up fetches = %d, want 1", fp.calls)
				}
			}
			land := tt.landing
			if land.err == nil {
				land.items = mkItems("1", "2", "3")
			}
			m.Update(land)
			if _, held := m.busy[session.Key("1")]; held == tt.wantClear {
				t.Errorf("held = %v after %+v, want %v", held, tt.landing, !tt.wantClear)
			}
		})
	}
}

func TestFailedRunUnlocksAtOnce(t *testing.T) {
	m, _ := busyModel(t)
	m.loading["p"] = true
	cmd := m.runAction("broken", busyActions["broken"])
	if len(m.busy) == 0 {
		t.Fatal("item not locked while the run is going")
	}
	d := finish(t, m, cmd)
	if d.ok {
		t.Fatalf("failing run reported ok: %+v", d)
	}
	if len(m.busy) != 0 {
		t.Errorf("busy after a failed run: %v", m.busy)
	}
}
