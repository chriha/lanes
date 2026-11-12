package ui

import (
	"errors"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/history"
	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/provider"
	"github.com/chriha/lanes/internal/session"
)

// autoStep is one thing that happens to an autoModel; now is the fixed clock the step may move.
type autoStep func(t *testing.T, m *Model, now *time.Time)

// labelled lists items by id; "2:auto-review" is item 2 with the label auto-review.
func labelled(specs ...string) []item.Item {
	var out []item.Item
	for _, s := range specs {
		id, label, _ := strings.Cut(s, ":")
		it := mkItems(id)[0]
		it.Fields["labels"] = []string{}
		if label != "" {
			it.Fields["labels"] = []string{label}
		}
		out = append(out, it)
	}
	return out
}

// land hands provider p's fetch result msg to the model and runs what follows.
func land(m *Model, msg fetchedMsg) {
	msg.name, msg.gen = "p", m.fetchStateOf("p").gen
	_, cmd := m.Update(msg)
	settle(m, cmd)
}

// full is a successful full fetch listing the items specs (see labelled).
func full(specs ...string) autoStep {
	return func(_ *testing.T, m *Model, _ *time.Time) { land(m, fetchedMsg{items: labelled(specs...)}) }
}

// quick is a partial (quick) fetch result listing specs.
func quick(specs ...string) autoStep {
	return func(_ *testing.T, m *Model, _ *time.Time) {
		land(m, fetchedMsg{items: labelled(specs...), partial: true})
	}
}

// failed is a full fetch that errors.
func failed() autoStep {
	return func(_ *testing.T, m *Model, _ *time.Time) { land(m, fetchedMsg{err: errors.New("boom")}) }
}

// wait moves the clock on by d.
func wait(d time.Duration) autoStep {
	return func(_ *testing.T, _ *Model, now *time.Time) { *now = now.Add(d) }
}

// colon runs a : command line.
func colon(line string) autoStep {
	return func(_ *testing.T, m *Model, _ *time.Time) {
		_, cmd := m.runCommand(line)
		settle(m, cmd)
	}
}

// poll lands a sessions poll that sees live (and nothing else).
func poll(live ...session.Info) autoStep {
	return func(_ *testing.T, m *Model, _ *time.Time) {
		_, cmd := m.Update(sessionsMsg{infos: live})
		m.polling = true // the follow-up polls must not reach tmux
		settle(m, cmd)
	}
}

// reload applies the same config again, as a config reload does.
func reload() autoStep {
	return func(_ *testing.T, m *Model, _ *time.Time) {
		m.Update(reloadedMsg{gen: m.reloadGen, cfg: m.cfg, providers: map[string]provider.Provider{"p": m.providers["p"]}})
		// its refetch is left unrun: the steps land the fetches
	}
}

// footerHas checks whether the footer shows text (want) or not.
func footerHas(text string, want bool) autoStep {
	return func(t *testing.T, m *Model, _ *time.Time) {
		t.Helper()
		if f := ansi.Strip(m.footer()); strings.Contains(f, text) != want {
			t.Errorf("footer %q shows %q: %v, want %v", f, text, !want, want)
		}
	}
}

// rowHas checks whether the list row of item id shows text (want) or not.
func rowHas(id, text string, want bool) autoStep {
	return func(t *testing.T, m *Model, _ *time.Time) {
		t.Helper()
		for _, it := range m.views[0].items {
			if it.ID != id {
				continue
			}
			if r := ansi.Strip(m.row(it, false, 4)); strings.Contains(r, text) != want {
				t.Errorf("row of %s %q shows %q: %v, want %v", id, r, text, !want, want)
			}
			return
		}
		t.Errorf("item %s is not listed", id)
	}
}

var newSession = regexp.MustCompile(`new-session -d -s (\S+)`)

// started lists the sessions the fake tmux was asked to create, in order.
func started(t *testing.T, log string) []string {
	t.Helper()
	raw, _ := os.ReadFile(log)
	var out []string
	for _, m := range newSession.FindAllStringSubmatch(string(raw), -1) {
		out = append(out, m[1])
	}
	return out
}

// A view's on_enter action starts on items that newly appear in it in a later full fetch: once per item, unless
// on_enter_repeat lets a re-joining item fire again after its cooldown; through the action's when, never on a busy
// or running item, never prompting or jumping, queued over max_claude_sessions.
func TestOnEnter(t *testing.T) {
	hour := time.Hour
	tests := []struct {
		name       string
		repeat     *time.Duration
		action     config.Action // the "review" action; default a plain session action
		limit      *int          // max_claude_sessions; default 0 (none)
		live       []session.Info
		earlier    []string // items an earlier lanes run auto-started review on
		steps      []autoStep
		wantStarts []string // sessions started, in order
		wantRuns   int      // headless runs started (history "started" events of item 2)
		wantNotice string   // substring of the final notice
	}{
		{name: "new arrival starts the action", steps: []autoStep{full("1"), full("1", "2")},
			wantStarts: []string{"review~2"}, wantNotice: "auto-started review on 2"},
		{name: "several arrivals start in one go", steps: []autoStep{full("1"), full("1", "2", "3")},
			wantStarts: []string{"review~2", "review~3"}, wantNotice: "auto-started review on 2 items"},
		{name: "startup: the first full fetch only seeds", steps: []autoStep{full("1", "2"), full("1", "2")}},
		{name: "config reload seeds again", steps: []autoStep{full("1"), reload(), full("1", "2"), full("1", "2")}},
		{name: "auto-started in an earlier run: no fire", earlier: []string{"2"},
			steps: []autoStep{full("1"), full("1", "2")}},
		{name: "plain refetch fires nothing more", steps: []autoStep{full("1"), full("1", "2"), full("1", "2")},
			wantStarts: []string{"review~2"}},
		{name: "re-entry without on_enter_repeat never fires again",
			steps:      []autoStep{full("1"), full("1", "2"), full("1"), wait(48 * time.Hour), full("1", "2")},
			wantStarts: []string{"review~2"}},
		{name: "re-entry inside the cooldown does not fire", repeat: &hour,
			steps:      []autoStep{full("1"), full("1", "2"), full("1"), wait(30 * time.Minute), full("1", "2")},
			wantStarts: []string{"review~2"}},
		{name: "re-entry after the cooldown fires again", repeat: &hour,
			steps:      []autoStep{full("1"), full("1", "2"), full("1"), wait(61 * time.Minute), full("1", "2")},
			wantStarts: []string{"review~2", "review~2"}},
		{name: "flapping starts at most one session per cooldown", repeat: &hour,
			steps: []autoStep{full("1"), full("1", "2"),
				full("1"), wait(10 * time.Minute), full("1", "2"), full("1"), wait(10 * time.Minute), full("1", "2"),
				full("1"), wait(10 * time.Minute), full("1", "2"), full("1"), wait(40 * time.Minute), full("1", "2"),
				full("1"), full("1", "2")},
			wantStarts: []string{"review~2", "review~2"}},
		{name: "with repeat, a startup item that re-joins fires", repeat: &hour,
			steps: []autoStep{full("1", "2"), full("1"), full("1", "2")}, wantStarts: []string{"review~2"}},
		{name: "when skips an item without the label, which stays seen",
			action:     config.Action{Run: "echo review", When: `"auto-review" in labels`},
			steps:      []autoStep{full("1"), full("1", "2", "3:auto-review"), full("1", "2:auto-review", "3:auto-review")},
			wantStarts: []string{"review~3"}},
		{name: "item already running the action is skipped",
			steps: []autoStep{full("1"), poll(session.Info{Name: "review~2", Action: "review", ItemKey: "2"}), full("1", "2")}},
		{name: ":auto off: arrivals meanwhile never fire",
			steps: []autoStep{full("1"), colon("auto off"), full("1", "2"), colon("auto on"), full("1", "2"),
				full("1", "2", "3")},
			wantStarts: []string{"review~3"}},
		{name: "failed headless run is not retried", action: config.Action{Type: "run", Run: "exit 3"},
			steps:    []autoStep{full("1"), full("1", "2"), full("1", "2"), full("1", "2")},
			wantRuns: 1, wantNotice: "auto: ✗ review failed"},
		{name: "no prompt and no jump, whatever confirm and jump say",
			action: config.Action{Run: "echo review", Confirm: "single", Jump: true},
			steps:  []autoStep{full("1"), full("1", "2", "3")}, wantStarts: []string{"review~2", "review~3"}},
		{name: "at the limit an arrival is queued, shown, and starts once there is room",
			limit: intp(1), live: others(1, false),
			steps: []autoStep{full("1"), full("1", "2"),
				footerHas("queued 1", true), rowHas("2", "queued review", true), poll(),
				footerHas("queued", false), rowHas("2", "queued review", false)},
			wantStarts: []string{"review~2"}},
		{name: "the queue drains first in, first out, within the limit",
			limit: intp(1), live: others(1, false),
			steps: []autoStep{full("1"), full("1", "2"), full("1", "2", "3"), footerHas("queued 2", true),
				poll(), poll(session.Info{Name: "review~2", Action: "review", ItemKey: "2"}), poll()},
			wantStarts: []string{"review~2", "review~3"}},
		{name: "a queued item that left the view is dropped",
			limit: intp(1), live: others(1, false),
			steps: []autoStep{full("1"), full("1", "2"), full("1"), footerHas("queued", false), poll(),
				full("1", "2")}},
		{name: "when is checked again before a queued start",
			action: config.Action{Run: "echo review", When: `"auto-review" in labels`},
			limit:  intp(1), live: others(1, false),
			steps: []autoStep{full("1"), full("1", "2:auto-review"), full("1", "2"), poll()}},
		{name: ":auto off holds the queue; arrivals meanwhile are not queued; :auto on drains it",
			limit: intp(1), live: others(1, false),
			steps: []autoStep{full("1"), full("1", "2"), colon("auto off"), poll(), full("1", "2", "3"),
				footerHas("queued 1", true), rowHas("3", "queued", false), colon("auto on"), poll()},
			wantStarts: []string{"review~2"}},
		{name: "a failed or partial fetch does not count as leaving", repeat: &hour,
			steps: []autoStep{full("1", "2"), failed(), quick("1"), full("1", "2")}},
		{name: "a partial fetch does not fire; the full one does",
			steps:      []autoStep{full("1"), quick("1", "2"), footerHas("auto-started", false), full("1", "2")},
			wantStarts: []string{"review~2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, log := limitModel(t, tt.limit, tt.live)
			if tt.limit == nil {
				m.cfg.Settings.MaxSessions = intp(0)
			}
			now := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
			old := clock
			clock = func() time.Time { return now }
			t.Cleanup(func() { clock = old })
			for _, id := range tt.earlier {
				history.Append(history.Event{Time: now.Add(-48 * time.Hour), Item: id, Action: "review", Event: "auto"})
			}
			act := tt.action
			if act.Run == "" {
				act = config.Action{Run: "echo review"}
			}
			if err := act.CompileWhen(); err != nil {
				t.Fatal(err)
			}
			m.cfg.Action = map[string]config.Action{"review": act}
			m.cfg.View = []config.View{{Name: "review", Provider: "p", Filter: "true", OnEnter: "review", OnEnterRepeat: tt.repeat}}
			m.ov = &config.Overrides{}
			m.applyBuilt(m.cfg, map[string]provider.Provider{"p": m.providers["p"]}, nil)
			m.data["p"] = nil // nothing listed before the first fetch

			for _, step := range tt.steps {
				step(t, m, &now)
				if m.mode == modeConfirm {
					t.Fatalf("an auto-run asked %q", m.confirm.prompt)
				}
			}

			if got := started(t, log); !reflect.DeepEqual(got, tt.wantStarts) {
				t.Errorf("started %v, want %v (notice %q)", got, tt.wantStarts, m.notice)
			}
			raw, _ := os.ReadFile(log)
			if strings.Contains(string(raw), "attach") || strings.Contains(string(raw), "switch-client") {
				t.Errorf("an auto-run jumped into its session: %s", raw)
			}
			runs := 0
			for _, e := range history.For("2") {
				if e.Event == "started" && act.Type == "run" {
					runs++
				}
			}
			if runs != tt.wantRuns {
				t.Errorf("headless runs on 2 = %d, want %d", runs, tt.wantRuns)
			}
			if !strings.Contains(m.notice, tt.wantNotice) {
				t.Errorf("notice = %q, want it to contain %q", m.notice, tt.wantNotice)
			}
		})
	}
}

// :auto reports the state of the on_enter actions and rejects other arguments.
func TestAutoCommand(t *testing.T) {
	tests := []struct {
		lines      []string
		wantNotice string
	}{
		{[]string{"auto"}, "auto-runs on, 0 queued"},
		{[]string{"auto off", "auto"}, "auto-runs off, 0 queued"},
		{[]string{"auto off", "auto on", "auto"}, "auto-runs on, 0 queued"},
		{[]string{"auto maybe"}, "usage: :auto [on|off]"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.lines, " / "), func(t *testing.T) {
			m, _ := busyModel(t)
			for _, l := range tt.lines {
				m.runCommand(l)
			}
			if m.notice != tt.wantNotice {
				t.Errorf("notice = %q, want %q", m.notice, tt.wantNotice)
			}
		})
	}
}
