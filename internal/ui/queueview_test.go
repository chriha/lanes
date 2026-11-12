package ui

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/provider"
)

// queueModel is a model with the views review (on_enter review) and other (after a divider), max_claude_sessions 1
// and one foreign live session: every arrival in review queues.
func queueModel(t *testing.T) (*Model, string) {
	t.Helper()
	m, log := limitModel(t, intp(1), others(1, false))
	m.cfg.Action = map[string]config.Action{"review": {Run: "echo review"}}
	m.cfg.View = []config.View{
		{Name: "review", Provider: "p", Filter: "true", OnEnter: "review"},
		{Name: "other", Provider: "p", Filter: "true", Divider: true},
	}
	m.ov = &config.Overrides{}
	m.applyBuilt(m.cfg, map[string]provider.Provider{"p": m.providers["p"]}, nil)
	m.data["p"] = nil
	now := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	old := clock
	clock = func() time.Time { return now }
	t.Cleanup(func() { clock = old })
	return m, log
}

// keys presses keys in normal mode.
func pressKeys(ks ...string) autoStep {
	return func(_ *testing.T, m *Model, _ *time.Time) { settle(m, press(m, ks...)) }
}

// viewIs checks the name of the current view.
func viewIs(name string) autoStep {
	return func(t *testing.T, m *Model, _ *time.Time) {
		t.Helper()
		if got := m.views[m.cur].cfg.Name; got != name {
			t.Errorf("current view = %q, want %q", got, name)
		}
	}
}

// queueIs checks the items the queue view lists, in order, and the queue behind it.
func queueIs(ids ...string) autoStep {
	return func(t *testing.T, m *Model, _ *time.Time) {
		t.Helper()
		qv := m.views[len(m.views)-1]
		if got := itemIDs(qv.items); !reflect.DeepEqual(got, append([]string{}, ids...)) {
			t.Errorf("queue view lists %v, want %v", got, ids)
		}
		var queued []string
		for _, e := range m.auto.queue {
			queued = append(queued, e.id)
		}
		if !reflect.DeepEqual(queued, append([]string(nil), ids...)) {
			t.Errorf("queue holds %v, want %v", queued, ids)
		}
	}
}

// tabsAre checks the view names in tab order.
func tabsAre(names ...string) autoStep {
	return func(t *testing.T, m *Model, _ *time.Time) {
		t.Helper()
		var got []string
		for _, v := range m.views {
			got = append(got, v.cfg.Name)
		}
		if !reflect.DeepEqual(got, names) {
			t.Errorf("tabs = %v, want %v", got, names)
		}
	}
}

// headerHas checks whether the tab bar shows text.
func headerHas(text string, want bool) autoStep {
	return func(t *testing.T, m *Model, _ *time.Time) {
		t.Helper()
		if h := ansi.Strip(m.header()); strings.Contains(h, text) != want {
			t.Errorf("header %q shows %q: %v, want %v", h, text, !want, want)
		}
	}
}

// listHas checks whether the rendered list shows text.
func listHas(text string, want bool) autoStep {
	return func(t *testing.T, m *Model, _ *time.Time) {
		t.Helper()
		if l := ansi.Strip(m.list()); strings.Contains(l, text) != want {
			t.Errorf("list %q shows %q: %v, want %v", l, text, !want, want)
		}
	}
}

// cursorOn checks the item under the cursor.
func cursorOn(id string) autoStep {
	return func(t *testing.T, m *Model, _ *time.Time) {
		t.Helper()
		if got := m.selectedID(); got != id {
			t.Errorf("cursor on %q, want %q", got, id)
		}
	}
}

// The built-in queue view is always the last tab, reached with Q, and lists the auto queue first in, first out
// with the action name on each row; x drops entries without touching sessions.
func TestQueueView(t *testing.T) {
	tests := []struct {
		name       string
		steps      []autoStep
		wantStarts []string
	}{
		{name: "last tab after views and dividers, muted without a count when empty",
			steps: []autoStep{tabsAre("review", "other", "queue"), headerHas("queue", true), headerHas("queue 0", false)}},
		{name: "the tab shows the number of queued entries",
			steps:      []autoStep{full("1"), full("1", "2", "3"), headerHas("queue 2", true), poll(), headerHas("queue 1", true)},
			wantStarts: []string{"review~2"}},
		{name: "the tab count ignores the search",
			steps: []autoStep{full("1"), full("1", "2", "3"), pressKeys("Q"),
				func(_ *testing.T, m *Model, _ *time.Time) { m.query = "zzz"; m.rebuild() }, headerHas("queue 2", true)}},
		{name: "Q selects it; 1-9 address configured views only",
			steps: []autoStep{pressKeys("Q"), viewIs("queue"), pressKeys("1"), viewIs("review"), pressKeys("2"), viewIs("other"),
				pressKeys("3"), viewIs("other"), pressKeys("Q"), viewIs("queue")}},
		{name: "l moves from the last configured view onto it",
			steps: []autoStep{pressKeys("2", "l"), viewIs("queue"), pressKeys("l"), viewIs("queue"), pressKeys("h"), viewIs("other")}},
		{name: "rows are the queued items in FIFO order with the action name",
			steps: []autoStep{full("1"), full("1", "4", "2", "3"), pressKeys("Q"), queueIs("4", "2", "3"),
				listHas("queued review", true)}},
		{name: "x drops the entry under the cursor and leaves sessions alone",
			steps: []autoStep{full("1"), full("1", "2", "3", "4"), pressKeys("Q", "j", "x"), queueIs("2", "4"), cursorOn("4"),
				footerHas("dropped 1 from the queue", true)}},
		{name: "x drops every marked entry",
			steps: []autoStep{full("1"), full("1", "2", "3", "4"), pressKeys("Q", " ", "j", "j", " ", "x"), queueIs("3")}},
		{name: "a dropped entry is not started when room frees up",
			steps: []autoStep{full("1"), full("1", "2", "3"), pressKeys("Q", "x"), poll()}, wantStarts: []string{"review~3"}},
		{name: "search narrows the list, not the queue",
			steps: []autoStep{full("1"), full("1", "2", "3"), pressKeys("Q"),
				func(t *testing.T, m *Model, _ *time.Time) {
					m.query = "3"
					m.rebuild()
					if got := itemIDs(m.items()); !reflect.DeepEqual(got, []string{"3"}) {
						t.Errorf("searched queue view lists %v, want [3]", got)
					}
					if len(m.auto.queue) != 2 {
						t.Errorf("queue = %v, want both entries", m.auto.queue)
					}
				}}},
		{name: "an empty queue shows an empty-state line and stays reachable",
			steps: []autoStep{pressKeys("Q"), viewIs("queue"), listHas("queue empty", true), listHas("nothing here", false)}},
		{name: "entries that drain update the list live",
			steps: []autoStep{full("1"), full("1", "2", "3"), pressKeys("Q"), queueIs("2", "3"),
				poll(), queueIs("3"), listHas("queued review", true)},
			wantStarts: []string{"review~2"}},
		{name: "entries whose item left the view drop out live",
			steps: []autoStep{full("1"), full("1", "2", "3"), pressKeys("Q"), queueIs("2", "3"), full("1", "3"), queueIs("3")}},
		{name: "action keys run nothing from the queue view",
			steps: []autoStep{full("1"), full("1", "2"), pressKeys("Q"), colon("run review"), queueIs("2"),
				footerHas("queue view runs no actions", true)}},
		{name: "a config reload keeps one queue view, last, and selected",
			steps: []autoStep{full("1"), full("1", "2"), pressKeys("Q"), reload(), tabsAre("review", "other", "queue"),
				viewIs("queue")}},
		{name: "a reload from another view keeps that view",
			steps: []autoStep{pressKeys("2"), reload(), tabsAre("review", "other", "queue"), viewIs("other")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, log := queueModel(t)
			now := clock()
			for _, step := range tt.steps {
				step(t, m, &now)
			}
			if got := started(t, log); !reflect.DeepEqual(got, tt.wantStarts) {
				t.Errorf("started %v, want %v", got, tt.wantStarts)
			}
			if raw, _ := os.ReadFile(log); strings.Contains(string(raw), "kill-session") {
				t.Errorf("a session was closed: %s", raw)
			}
			if m.mode == modeConfirm {
				t.Errorf("a confirmation is open: %q", m.confirm.prompt)
			}
		})
	}
}
