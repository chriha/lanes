package ui

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/filter"
	"github.com/chriha/lanes/internal/history"
	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/session"
)

// A session action whose session is seen dying starts its on_success (exit 0) or on_failure (else) action on the
// same item through the normal start path; killed and interrupted sessions start nothing.
func TestChainOnSessionExit(t *testing.T) {
	never, err := filter.Compile("false")
	if err != nil {
		t.Fatal(err)
	}
	chained := config.Action{Run: "echo work", OnSuccess: "review", OnFailure: "fix"}
	tests := []struct {
		name        string
		action      config.Action // the "work" action
		key         string        // item of the work session
		exit        int
		gone        bool   // the session vanished instead of dying (closed with x)
		interrupted bool   // X interrupted the session before it died
		busy        string // action holding the item when the session dies
		running     bool   // the follow-up's session is already alive on the item
		gated       bool   // the follow-up's when matches no item
		wantNotice  string // "" = the notice stays untouched
		wantStart   string // follow-up session started; "" = none
	}{
		{name: "exit 0 starts on_success", action: chained, key: "1",
			wantNotice: "work finished on 1 → starting review", wantStart: "review~1"},
		{name: "non-zero exit starts on_failure", action: chained, key: "1", exit: 2,
			wantNotice: "work failed on 1 → starting fix", wantStart: "fix~1"},
		{name: "no follow-up for the outcome", action: config.Action{Run: "echo work", OnFailure: "fix"}, key: "1"},
		{name: "killed session starts nothing", action: chained, key: "1", gone: true},
		{name: "interrupted session starts nothing", action: chained, key: "1", exit: 143, interrupted: true},
		{name: "item no longer listed", action: chained, key: "9",
			wantNotice: "work finished on 9 → review: 9 no longer listed"},
		{name: "busy item is skipped", action: chained, key: "1", busy: "quick",
			wantNotice: "work finished on 1 → review: 1 busy (quick)"},
		{name: "follow-up already running", action: chained, key: "1", running: true,
			wantNotice: "work finished on 1 → review: review already running"},
		{name: "when skips the item", action: chained, key: "1", gated: true,
			wantNotice: "work finished on 1 → review: skipped 1 item(s) (when)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := busyModel(t)
			log := paneTmux(t, 0)
			mgr, err := session.NewManager()
			if err != nil {
				t.Fatal(err)
			}
			m.sess = mgr
			review := config.Action{Run: "echo review"}
			if tt.gated {
				review.When, review.WhenFilter = "false", never
			}
			m.cfg = &config.Config{Action: map[string]config.Action{
				"work": tt.action, "review": review, "fix": {Run: "echo fix"}, "quick": {Type: "run", Run: "true"},
			}}
			m.cfg.Settings.NoNotify = true
			if tt.busy != "" {
				m.busy[session.Key(tt.key)] = busyState{action: tt.busy}
			}
			alive := session.Info{Name: "work~" + tt.key, Action: "work", ItemKey: tt.key}
			before := []session.Info{alive}
			var after []session.Info
			if !tt.gone {
				dead := alive
				dead.Dead, dead.Exit = true, tt.exit
				after = append(after, dead)
			}
			if tt.running {
				r := session.Info{Name: "review~" + tt.key, Action: "review", ItemKey: tt.key}
				before, after = append(before, r), append(after, r)
			}
			if tt.interrupted {
				m.interrupted = map[string]bool{alive.Name: true}
			}
			m.sessions = before
			m.notice = ""
			_, cmd := m.Update(sessionsMsg{infos: after})
			if m.notice != tt.wantNotice {
				t.Errorf("notice = %q, want %q", m.notice, tt.wantNotice)
			}
			runAll(cmd)
			raw, _ := os.ReadFile(log)
			var started []string
			for _, l := range strings.Split(string(raw), "\n") {
				if _, rest, ok := strings.Cut(l, "new-session -d -s "); ok {
					started = append(started, strings.Fields(rest)[0])
				}
			}
			if strings.Join(started, ",") != tt.wantStart {
				t.Errorf("started %q, want %q\ntmux calls:\n%s", started, tt.wantStart, raw)
			}
			if tt.wantStart != "" {
				evs := history.For(tt.key)
				want := strings.TrimSuffix(tt.wantStart, "~"+tt.key)
				if len(evs) == 0 || evs[len(evs)-1].Event != "started" || evs[len(evs)-1].Action != want {
					t.Errorf("history = %+v, want a last %s started event", evs, want)
				}
			}
		})
	}
}

// A run action starts, per item, its on_success or on_failure action on that item once the run is done; a refresh
// run's hold until the refetch does not keep the follow-up from starting.
func TestChainOnRunResult(t *testing.T) {
	tests := []struct {
		name       string
		action     config.Action // the "check" run action
		items      []string
		wantNotice string
		wantStart  string // follow-up sessions started, in order; "" = none
		wantHeld   bool   // item 1 stays busy until the refetch
	}{
		{name: "success starts on_success", items: []string{"1"},
			action:     config.Action{Type: "run", Run: "true", OnSuccess: "review", OnFailure: "fix"},
			wantNotice: "check finished on 1 → starting review", wantStart: "review~1"},
		{name: "failure starts on_failure", items: []string{"1"},
			action:     config.Action{Type: "run", Run: "exit 3", OnSuccess: "review", OnFailure: "fix"},
			wantNotice: "check failed on 1 → starting fix", wantStart: "fix~1"},
		{name: "each item chains on its own outcome", items: []string{"1", "2"},
			action:     config.Action{Type: "run", Run: "test {{.id}} = 1", OnSuccess: "review", OnFailure: "fix"},
			wantNotice: "check failed on 2 → starting fix", wantStart: "review~1,fix~2"},
		{name: "refresh run still chains", items: []string{"1"},
			action:     config.Action{Type: "run", Run: "true", Refresh: true, OnSuccess: "review"},
			wantNotice: "check finished on 1 → starting review", wantStart: "review~1"},
		{name: "no follow-up for the outcome", items: []string{"1"},
			action:     config.Action{Type: "run", Run: "true", OnFailure: "fix"},
			wantNotice: "✓ check done on 1 item(s)"},
		{name: "refresh run without a follow-up keeps its hold", items: []string{"1"},
			action:     config.Action{Type: "run", Run: "true", Refresh: true, OnFailure: "fix"},
			wantNotice: "✓ check done on 1 item(s)", wantHeld: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := busyModel(t)
			log := paneTmux(t, 0)
			mgr, err := session.NewManager()
			if err != nil {
				t.Fatal(err)
			}
			m.sess = mgr
			m.cfg = &config.Config{Action: map[string]config.Action{
				"check": tt.action, "review": {Run: "echo review"}, "fix": {Run: "echo fix"},
			}}
			m.cfg.Settings.NoNotify = true
			var its []item.Item
			for _, it := range m.data["p"] {
				if slices.Contains(tt.items, it.ID) {
					its = append(its, it)
				}
			}
			cmd := m.runActionOn("check", tt.action, its)
			if m.mode == modeConfirm {
				cmd = press(m, "y")
			}
			if cmd == nil {
				t.Fatalf("no command returned (notice %q)", m.notice)
			}
			_, next := m.Update(cmd())
			if m.notice != tt.wantNotice {
				t.Errorf("notice = %q, want %q", m.notice, tt.wantNotice)
			}
			if b, held := m.busy[session.Key("1")]; (held && b.action == "check") != tt.wantHeld {
				t.Errorf("item 1 held by check = %v, want %v", held && b.action == "check", tt.wantHeld)
			}
			runAll(next)
			raw, _ := os.ReadFile(log)
			var started []string
			for _, l := range strings.Split(string(raw), "\n") {
				if _, rest, ok := strings.Cut(l, "new-session -d -s "); ok {
					started = append(started, strings.Fields(rest)[0])
				}
			}
			if strings.Join(started, ",") != tt.wantStart {
				t.Errorf("started %q, want %q\ntmux calls:\n%s", started, tt.wantStart, raw)
			}
		})
	}
}
