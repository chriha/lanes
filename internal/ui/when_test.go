package ui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/session"
)

// whenModel lists item 1 (ticket), 2 (pr) and 3 (ticket), with actions scoped by when.
func whenModel(t *testing.T) *Model {
	t.Helper()
	m, _ := busyModel(t)
	m.cfg.Action = map[string]config.Action{
		"tkt":  {Type: "run", Run: "true", When: `kind == "ticket"`},
		"prx":  {Type: "run", Run: "true", When: `kind == "pr"`},
		"anyx": {Type: "run", Run: "true"},
		"oops": {Type: "run", Run: "true", When: `kind + 1 == 2`}, // errors at runtime on a string kind
	}
	for name, a := range m.cfg.Action {
		if err := a.CompileWhen(); err != nil {
			t.Fatal(err)
		}
		m.cfg.Action[name] = a
	}
	m.cfg.Keymap = map[string]string{"T": "tkt", "P": "prx", "A": "anyx"}
	items := mkItems("1", "2", "3")
	for i, kind := range []string{"ticket", "pr", "ticket"} {
		items[i].Fields["kind"] = kind
	}
	m.data["p"] = items
	m.rebuild()
	return m
}

func TestRunActionHonoursWhen(t *testing.T) {
	tests := []struct {
		name       string
		action     string
		marks      []string
		cursor     int
		byCommand  bool     // :run instead of calling runAction directly
		wantPrompt string   // confirm prompt; empty = none expected
		wantRan    []string // item IDs the action ran on; nil = nothing started
		wantNotice string   // substring of the final notice
		wantMarked []string // marks left after the run
	}{
		{
			name: "all match", action: "tkt", marks: []string{"1", "3"},
			wantPrompt: `Run "tkt" on 2 items (1, 3)?`, wantRan: []string{"1", "3"},
			wantNotice: "✓ tkt done on 2 item(s)", wantMarked: []string{},
		},
		{
			name: "partial match", action: "tkt", marks: []string{"1", "2", "3"},
			wantPrompt: `Run "tkt" on 2 items (1, 3)?`, wantRan: []string{"1", "3"},
			wantNotice: "skipped 1 item(s) (when)", wantMarked: []string{"2"},
		},
		{
			name: "partial match leaves one item, no multi-item prompt", action: "prx", marks: []string{"1", "2"},
			wantRan: []string{"2"}, wantNotice: "skipped 1 item(s) (when)", wantMarked: []string{"1"},
		},
		{
			name: "no match", action: "prx", marks: []string{"1", "3"},
			wantNotice: "prx: skipped 2 item(s) (when)", wantMarked: []string{"1", "3"},
		},
		{
			name: "no match by :run on the cursor item", action: "tkt", cursor: 1, byCommand: true,
			wantNotice: "tkt: skipped 1 item(s) (when)", wantMarked: []string{},
		},
		{
			name: "match by :run on the cursor item", action: "prx", cursor: 1, byCommand: true,
			wantRan: []string{"2"}, wantNotice: "✓ prx done on 1 item(s)", wantMarked: []string{},
		},
		{
			name: "runtime error counts as no match and is reported", action: "oops", cursor: 0,
			wantNotice: "oops: skipped 1 item(s) (when", wantMarked: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := whenModel(t)
			m.cursor = tt.cursor
			for _, id := range tt.marks {
				m.marks[id] = true
			}
			var cmd tea.Cmd
			if tt.byCommand {
				_, cmd = m.runCommand("run " + tt.action)
			} else {
				cmd = m.runAction(tt.action, m.cfg.Action[tt.action])
			}
			if tt.wantPrompt != "" {
				if m.confirm == nil {
					t.Fatalf("no confirm prompt, notice %q", m.notice)
				}
				if m.confirm.prompt != tt.wantPrompt {
					t.Errorf("prompt = %q, want %q", m.confirm.prompt, tt.wantPrompt)
				}
				cmd = m.confirm.do()
			} else if m.confirm != nil {
				t.Fatalf("unexpected prompt %q", m.confirm.prompt)
			}
			if tt.wantRan == nil {
				if cmd != nil {
					t.Fatal("an action started although no item matches")
				}
				if len(m.busy) != 0 {
					t.Errorf("items locked although nothing ran: %v", m.busy)
				}
			} else {
				d := finish(t, m, cmd)
				want := make([]string, len(tt.wantRan))
				for i, id := range tt.wantRan {
					want[i] = session.Key(id)
				}
				if !reflect.DeepEqual(d.keys, want) {
					t.Errorf("ran on %v, want %v", d.keys, want)
				}
			}
			if !strings.Contains(m.notice, tt.wantNotice) {
				t.Errorf("notice = %q, want it to contain %q", m.notice, tt.wantNotice)
			}
			if tt.wantPrompt != "" && !strings.Contains(tt.wantNotice, "skipped") && strings.Contains(m.notice, "skipped") {
				t.Errorf("notice %q reports skips although all items matched", m.notice)
			}
			got := []string{}
			for _, id := range []string{"1", "2", "3"} {
				if m.marks[id] {
					got = append(got, id)
				}
			}
			if !reflect.DeepEqual(got, tt.wantMarked) {
				t.Errorf("marked = %v, want %v", got, tt.wantMarked)
			}
		})
	}
}

func TestHelpListsActionsMatchingCursorItem(t *testing.T) {
	tests := []struct {
		name   string
		cursor int
		empty  bool // view without items
		want   []string
		absent []string
	}{
		{"ticket under cursor", 0, false, []string{"tkt", "anyx"}, []string{"prx"}},
		{"pr under cursor", 1, false, []string{"prx", "anyx"}, []string{"tkt"}},
		{"empty list shows all", 0, true, []string{"tkt", "prx", "anyx"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := whenModel(t)
			if tt.empty {
				m.data["p"] = nil
				m.rebuild()
			}
			m.cursor = tt.cursor
			help := ansi.Strip(strings.Join(m.helpLines(), "\n"))
			for _, name := range tt.want {
				if !strings.Contains(help, " "+name+" ") {
					t.Errorf("help lacks action %q", name)
				}
			}
			for _, name := range tt.absent {
				if strings.Contains(help, " "+name+" ") {
					t.Errorf("help lists action %q that does not match the cursor item", name)
				}
			}
		})
	}
}

func TestMatchWhenUsesStoredFilter(t *testing.T) {
	act := config.Action{When: `kind == "ticket"`}
	if err := act.CompileWhen(); err != nil {
		t.Fatal(err)
	}
	act.When = `kind ===` // would fail to compile; matchWhen must use the stored filter instead
	its := mkItems("1", "2")
	its[0].Fields["kind"] = "ticket"
	its[1].Fields["kind"] = "pr"
	match, skip, err := matchWhen(act, its)
	if err != nil {
		t.Fatal(err)
	}
	if len(match) != 1 || match[0].ID != "1" || len(skip) != 1 || skip[0].ID != "2" {
		t.Errorf("match = %v, skip = %v; want item 1 matched and 2 skipped", match, skip)
	}
}
