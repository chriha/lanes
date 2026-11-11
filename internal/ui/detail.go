package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/item"
)

// detailEntry is the fetched detail of one item, or the fetch still running for it.
type detailEntry struct {
	fp      string // fingerprint of the item the fetch started for
	loading bool
	d       item.Detail
	err     error
}

// detailMsg delivers a detail fetch started by showDetail.
type detailMsg struct {
	key, fp string
	d       item.Detail
	err     error
}

// showDetail opens the detail screen for the item under the cursor; it does nothing without one. When the item's
// provider can fetch details and none is cached for the item as it is now, it returns the fetch.
func (m *Model) showDetail() tea.Cmd {
	if it := m.items(); m.cursor < len(it) {
		m.detail = it[m.cursor]
		m.mode, m.helpOff = modeDetail, 0
	}
	return nil
}

// onDetail stores a landed detail fetch unless its item changed since it started.
func (m *Model) onDetail(msg detailMsg) {}

// detailLines stacks the detail screen's sections: a header with the item's id, title, URL, author and its
// pipeline/draft state, then Fields with the note and every remaining field untruncated (the ci_jobs one job per
// line), keys in one column as wide as the longest key and wrapped values indented to the value column.
func (m *Model) detailLines() []string {
	it := m.detail
	w := max(m.width, 1)
	var out []string
	add := func(l string) { out = append(out, strings.Split(ansi.Wrap(l, w, ""), "\n")...) }
	add(" " + sID.Render(it.ID) + "  " + sTitle.Render(it.Title))
	add(" " + sDim.Render(it.URL))
	state := sDim.Render("by ") + sText.Render(it.Author)
	if ci, ok := it.Fields["ci"]; ok {
		state += sDim.Render(" · ci ") + sText.Render(fmtVal(ci))
	}
	if it.Draft {
		state += sDim.Render(" · ") + sWarn.Render("draft")
	}
	add(" " + state)

	type row struct {
		key  string
		vals []string
	}
	var rows []row
	note := m.notes.Get(it.ID)
	if note != "" {
		rows = append(rows, row{"note", strings.Split(note, "\n")})
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
		if k == "ci" || k == "note" && v == any(note) {
			continue // shown above
		}
		if k != "ci_jobs" {
			rows = append(rows, row{k, []string{fmtVal(v)}})
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
		if len(jobs) > 0 {
			rows = append(rows, row{k, jobs})
		}
	}
	if len(rows) == 0 {
		return out
	}
	kw := 0
	for _, r := range rows {
		kw = max(kw, ansi.StringWidth(r.key))
	}
	out = append(out, "", " "+sTitle.Render("Fields"))
	for _, r := range rows {
		label := r.key
		for _, v := range r.vals {
			for _, l := range strings.Split(ansi.Wrap(v, max(w-kw-3, 1), ""), "\n") {
				out = append(out, " "+sDim.Render(label+strings.Repeat(" ", kw-ansi.StringWidth(label)))+"  "+sText.Render(l))
				label = ""
			}
		}
	}
	return out
}
