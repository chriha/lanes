package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// showDetail opens the detail screen for the item under the cursor; it does nothing without one.
func (m *Model) showDetail() {
	if it := m.items(); m.cursor < len(it) {
		m.detail = it[m.cursor]
		m.mode, m.helpOff = modeDetail, 0
	}
}

// detailLines lists the detail item's title, URL, author, note and every field untruncated, the ci_jobs one job
// per line, wrapped at the screen width.
func (m *Model) detailLines() []string {
	it := m.detail
	row := func(k, v string) string { return " " + sDim.Render(fmt.Sprintf("%-12s", k)) + sText.Render(v) }
	lines := []string{" " + sID.Render(it.ID) + "  " + sTitle.Render(it.Title), " " + sDim.Render(it.URL), "", row("author", it.Author)}
	note := m.notes.Get(it.ID)
	if note != "" {
		label := "note"
		for _, l := range strings.Split(note, "\n") {
			lines = append(lines, row(label, l))
			label = ""
		}
	}
	// the preview's order first, then the remaining fields by name
	keys, seen := []string{}, map[string]bool{}
	for _, k := range preferred {
		if _, ok := it.Fields[k]; ok {
			keys, seen[k] = append(keys, k), true
		}
	}
	var rest []string
	for k := range it.Fields {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range append(keys, rest...) {
		v := it.Fields[k]
		if k == "note" && v == any(note) {
			continue // shown above
		}
		if k != "ci_jobs" {
			lines = append(lines, row(k, fmtVal(v)))
			continue
		}
		var jobs []string
		switch x := v.(type) {
		case []string:
			jobs = x
		case []any: // decoded from JSON (cache, command provider)
			for _, e := range x {
				jobs = append(jobs, fmt.Sprint(e))
			}
		default:
			jobs = []string{fmtVal(v)}
		}
		label := k
		for _, j := range jobs {
			lines = append(lines, row(label, j))
			label = ""
		}
	}
	var out []string
	for _, l := range lines {
		out = append(out, strings.Split(ansi.Wrap(l, max(m.width, 1), ""), "\n")...)
	}
	return out
}
