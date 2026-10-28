package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/history"
	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/session"
)

const previewH = 9

var preferred = []string{"iid", "number", "key", "branch", "target", "ticket", "labels", "reviewers", "assignees", "approved_by", "ci", "diverged", "open_threads"}

func fmtVal(v any) string {
	switch x := v.(type) {
	case []string:
		return strings.Join(x, ", ")
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = fmt.Sprint(e)
		}
		return strings.Join(parts, ", ")
	}
	return fmt.Sprint(v)
}

// preview renders exactly previewH lines of detail for the item under the cursor.
func (m *Model) preview() string {
	lines := make([]string, 0, previewH)
	if it := m.items(); m.cursor < len(it) {
		cur := it[m.cursor]
		lines = append(lines, " "+sID.Render(cur.ID)+"  "+sTitle.Render(cur.Title), " "+sDim.Render(cur.URL))
		keys := []string{}
		for _, k := range preferred {
			if _, ok := cur.Fields[k]; ok {
				keys = append(keys, k)
			}
		}
		if len(keys) == 0 {
			for k := range cur.Fields {
				keys = append(keys, k)
			}
			sort.Strings(keys)
		}
		lines = append(lines, " "+sDim.Render(fmt.Sprintf("%-12s", "author"))+sText.Render(cur.Author))
		note := m.notes.Get(cur.ID)
		if note != "" {
			label := "note"
			if v, ok := cur.Fields["note"]; ok && v != any(note) {
				label = "my note" // the provider's note field is listed below under "note"
			}
			for _, l := range strings.Split(note, "\n") {
				lines = append(lines, " "+sDim.Render(fmt.Sprintf("%-12s", label))+sText.Render(l))
				label = ""
			}
		}
		for _, k := range keys {
			if k == "note" && cur.Fields[k] == any(note) {
				continue // shown above
			}
			if v := fmtVal(cur.Fields[k]); v != "" && v != "[]" && v != "0" {
				lines = append(lines, " "+sDim.Render(fmt.Sprintf("%-12s", k))+sText.Render(v))
			}
		}
		lines = append(lines, m.previewSessions(cur)...)
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], m.width, "…")
	}
	for len(lines) < previewH {
		lines = append(lines, "")
	}
	return strings.Join(lines[:previewH], "\n") + "\n"
}

func (m *Model) previewSessions(it item.Item) []string {
	var out []string
	for _, s := range m.sessionsOf(it) {
		b := s.Badge()
		out = append(out, " "+sDim.Render(fmt.Sprintf("%-12s", "session"))+badgeStyle(b.Kind).Render(b.Text)+sDim.Render("  since "+s.Created.Format("15:04")))
	}
	evs := history.For(session.Key(it.ID))
	if n := len(evs); n > 0 {
		e := evs[n-1]
		out = append(out, " "+sDim.Render(fmt.Sprintf("%-12s", "last run"))+sText.Render(fmt.Sprintf("%s %s  %s", e.Action, e.Event, e.Time.Format("01-02 15:04"))))
	}
	return out
}
