package ui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/snooze"
)

// tabCount is the item count a view's tab shows.
func tabCount(m *Model, name string) int {
	for _, v := range m.views {
		if v.cfg.Name == name {
			return len(v.items)
		}
	}
	return -1
}

func storedSnoozes(t *testing.T) []string {
	t.Helper()
	s, err := snooze.Load()
	if err != nil {
		t.Fatal(err)
	}
	return keys(s)
}

func keys(s snooze.Store) []string {
	out := []string{}
	for id := range s {
		out = append(out, id)
	}
	return out
}

func TestSnoozeHidesFromEveryView(t *testing.T) {
	tests := []struct {
		name   string
		marks  []string
		input  string
		hidden []string
	}{
		{"timed, cursor item", nil, "2h", []string{"a"}},
		{"until it changes, cursor item", nil, "", []string{"a"}},
		{"marked items", []string{"a", "b"}, "3d", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newNotesModel(t)
			m.data["p"] = mkItems("a", "b", "c")
			m.rebuild()
			for _, id := range tt.marks {
				m.marks[id] = true
			}
			command(m, strings.TrimSpace("snooze "+tt.input))
			want := []string{"a", "b", "c"}[len(tt.hidden):]
			if got := itemIDs(m.items()); !reflect.DeepEqual(got, want) {
				t.Errorf("all view = %v, want %v", got, want)
			}
			if n := tabCount(m, "one"); n != 0 {
				t.Errorf("view one still counts %d items", n)
			}
			if got := storedSnoozes(t); len(got) != len(tt.hidden) {
				t.Errorf("stored snoozes = %v, want %v", got, tt.hidden)
			}
			if len(m.marks) != 0 {
				t.Errorf("marks kept after snoozing: %v", m.marks)
			}
			if f, want := ansi.Strip(m.footer()), fmt.Sprintf("%d snoozed", len(tt.hidden)); !strings.Contains(f, want) {
				t.Errorf("footer %q lacks %q", f, want)
			}
		})
	}
}

func TestSnoozeInvalidInput(t *testing.T) {
	for _, in := range []string{"3x", "2020-01-01"} {
		t.Run(in, func(t *testing.T) {
			m := newNotesModel(t)
			command(m, "snooze "+in)
			if !strings.Contains(m.notice, "snooze") {
				t.Errorf("notice = %q, want an error", m.notice)
			}
			if got := itemIDs(m.items()); !reflect.DeepEqual(got, []string{"a", "b"}) {
				t.Errorf("items = %v, nothing should be snoozed", got)
			}
			if got := storedSnoozes(t); len(got) != 0 {
				t.Errorf("stored snoozes = %v", got)
			}
		})
	}
}

func TestSnoozeWakes(t *testing.T) {
	changed := mkItems("a", "b")
	changed[0].Title = "a, retitled"
	tests := []struct {
		name  string
		input string
		msg   func() any
		woken bool
	}{
		{"timed: tick before the time", "2h", func() any { return tickMsg(time.Now().Add(time.Hour)) }, false},
		{"timed: tick after the time", "2h", func() any { return tickMsg(time.Now().Add(3 * time.Hour)) }, true},
		{"timed: changed data does not wake", "2h", func() any { return fetchedMsg{name: "p", items: changed} }, false},
		{"change: identical fetch keeps it", "", func() any { return fetchedMsg{name: "p", items: mkItems("a", "b")} }, false},
		{"change: changed fetch wakes it", "", func() any { return fetchedMsg{name: "p", items: changed} }, true},
		{"change: partial result never wakes", "", func() any {
			return fetchedMsg{name: "p", partial: true, items: []item.Item{{ID: "a", Title: "a", Fields: map[string]any{}, Pending: []string{"ci"}}}}
		}, false},
		{"change: the item missing from a fetch keeps it", "", func() any { return fetchedMsg{name: "p", items: mkItems("b")} }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newNotesModel(t)
			command(m, strings.TrimSpace("snooze "+tt.input))
			m.Update(tt.msg())
			if m.data["p"][0].ID != "a" {
				m.data["p"] = append(mkItems("a"), m.data["p"]...) // a left the fetch: put it back to see whether it shows
				m.rebuild()
			}
			shown := false
			for _, it := range m.items() {
				shown = shown || it.ID == "a"
			}
			if shown != tt.woken {
				t.Errorf("a shown = %v, want %v", shown, tt.woken)
			}
			if stored := len(storedSnoozes(t)) == 1; stored == tt.woken {
				t.Errorf("snooze still stored = %v after waking = %v", stored, tt.woken)
			}
		})
	}
}

func TestShowSnoozedAndUnsnooze(t *testing.T) {
	m := newNotesModel(t)
	command(m, "snooze 2h")
	if f := ansi.Strip(m.footer()); !strings.Contains(f, "1 snoozed") {
		t.Errorf("footer %q lacks the snoozed count", f)
	}
	command(m, "snoozed")
	if got := itemIDs(m.items()); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf(":snoozed shows %v, want a and b", got)
	}
	if row := rowOf(t, m, "a"); !strings.Contains(row, " z") {
		t.Errorf("snoozed row %q lacks the z marker", row)
	}
	if row := rowOf(t, m, "b"); strings.Contains(row, " z") {
		t.Errorf("awake row %q has the z marker", row)
	}
	m.cursor = 0
	nav(m, "z")
	if m.mode != modeNormal {
		t.Errorf("z on a snoozed item opened the prompt")
	}
	if got := storedSnoozes(t); len(got) != 0 {
		t.Fatalf("z did not unsnooze: %v", got)
	}
	if f := ansi.Strip(m.footer()); strings.Contains(f, "1 snoozed") {
		t.Errorf("footer %q shows a snoozed count of zero", f)
	}
	command(m, "snoozed")
	if got := itemIDs(m.items()); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("after unsnoozing items = %v", got)
	}
}

func TestZOpensThePrompt(t *testing.T) {
	m := newNotesModel(t)
	nav(m, "z")
	if m.mode != modeCommand || m.cmdline.Value() != "snooze " {
		t.Errorf("mode = %v, cmdline = %q; want the snooze prompt", m.mode, m.cmdline.Value())
	}
}

func TestSnoozedField(t *testing.T) {
	m := newNotesModel(t)
	command(m, "snooze 2h")
	command(m, "filter snoozed")
	if len(m.items()) != 0 {
		t.Errorf("hidden by default, even when the filter asks for snoozed: %v", itemIDs(m.items()))
	}
	command(m, "snoozed")
	if got := itemIDs(m.items()); !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("filter snoozed with :snoozed = %v, want [a]", got)
	}
	command(m, "filter !snoozed")
	if got := itemIDs(m.items()); !reflect.DeepEqual(got, []string{"b"}) {
		t.Errorf("filter !snoozed = %v, want [b]", got)
	}
}

func TestZIsBuiltIn(t *testing.T) {
	if !strings.Contains(builtinKeys, "z") {
		t.Error("z is not in builtinKeys")
	}
}
