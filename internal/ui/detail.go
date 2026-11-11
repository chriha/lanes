package ui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/provider"
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
	its := m.items()
	if m.cursor >= len(its) {
		return nil
	}
	it := its[m.cursor]
	m.detail, m.detailKey = it, ""
	m.mode, m.helpOff = modeDetail, 0
	name := m.views[m.cur].cfg.Provider
	p, ok := m.providers[name].(provider.Detailer)
	if !ok {
		return nil
	}
	key, fp := name+"\x00"+it.ID, it.Fingerprint()
	m.detailKey = key
	if e := m.details[key]; e != nil && e.fp == fp && (e.loading || e.err == nil || errors.Is(e.err, provider.ErrNoDetail)) {
		return nil // fetched or fetching for the item as it is now; a failed fetch is retried
	}
	if m.details == nil {
		m.details = map[string]*detailEntry{}
	}
	m.details[key] = &detailEntry{fp: fp, loading: true}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		d, err := p.Detail(ctx, it)
		return detailMsg{key: key, fp: fp, d: d, err: err}
	}
}

// onDetail stores a landed detail fetch unless its item changed since it started.
func (m *Model) onDetail(msg detailMsg) {
	if e := m.details[msg.key]; e != nil && e.fp == msg.fp {
		*e = detailEntry{fp: msg.fp, d: msg.d, err: msg.err}
	}
}

// jobIcons marks a pipeline job's status; any other status gets "·".
var jobIcons = map[string]string{"success": "✓", "failed": "✗", "warning": "⚠", "running": "●", "pending": "○", "created": "○", "skipped": "–", "canceled": "–", "manual": "▸"}

// detailSections renders the fetched sections of the shown item (Description, Pipeline, Threads, Comments), a
// loading line while the fetch runs or an error line when it failed; nothing when its provider fetches none.
func (m *Model) detailSections(w int) []string {
	e := m.details[m.detailKey]
	if m.detailKey == "" || e == nil || errors.Is(e.err, provider.ErrNoDetail) {
		return nil
	}
	if e.loading {
		return []string{"", " " + sDim.Render("loading details…")}
	}
	if e.err != nil {
		return []string{"", " " + sErr.Render("details: "+e.err.Error())}
	}
	var out []string
	// add wraps text to the screen with every line indented by ind
	add := func(ind int, text string, st lipgloss.Style) {
		pad := strings.Repeat(" ", ind)
		for _, l := range strings.Split(ansi.Wrap(text, max(w-ind, 1), ""), "\n") {
			out = append(out, pad+st.Render(l))
		}
	}
	title := func(t string) { out = append(out, "", " "+sTitle.Render(t)) }
	now := time.Now()
	// by is a comment's author and age, leaving out what is unknown
	by := func(c item.Comment) string {
		parts := []string{}
		if c.Author != "" {
			parts = append(parts, c.Author)
		}
		if !c.Created.IsZero() {
			parts = append(parts, age(max(now.Sub(c.Created), 0)))
		}
		return strings.Join(parts, " · ")
	}
	d := e.d
	if d.Description != "" {
		title("Description")
		add(1, d.Description, sText)
	}
	if pl := d.Pipeline; pl != nil {
		title(fmt.Sprintf("Pipeline #%s · %s · %s", pl.ID, pl.Status, pl.Duration.Round(time.Second)))
		var stages []string
		jobs := map[string][]item.Job{}
		for _, j := range pl.Jobs {
			if _, ok := jobs[j.Stage]; !ok {
				stages = append(stages, j.Stage)
			}
			jobs[j.Stage] = append(jobs[j.Stage], j)
		}
		for _, st := range stages {
			ind := 1 // jobs without a stage (GitHub checks) go without a stage line
			if st != "" {
				add(1, st, sDim)
				ind = 3
			}
			for _, j := range jobs[st] {
				icon, ok := jobIcons[j.Status]
				if !ok {
					icon = "·"
				}
				l := sText.Render(icon + " " + j.Name)
				if j.Duration > 0 {
					l += "  " + sDim.Render(j.Duration.Round(time.Second).String())
				}
				add(ind, l, lipgloss.NewStyle())
			}
		}
	}
	if len(d.Threads) > 0 {
		title(fmt.Sprintf("Threads (%d)", len(d.Threads)))
		for _, th := range d.Threads {
			head := "● open"
			if th.Resolved {
				head = "✓ resolved"
			}
			if th.File != "" {
				head += fmt.Sprintf(" · %s:%d", th.File, th.Line)
			}
			if len(th.Comments) > 0 {
				head += " · " + by(th.Comments[0])
			}
			add(1, head, sText)
			for i, c := range th.Comments {
				if i > 0 {
					add(5, by(c), sDim)
					add(5, c.Body, sText)
				} else {
					add(3, c.Body, sText)
				}
			}
		}
	}
	if len(d.Comments) > 0 {
		title(fmt.Sprintf("Comments (%d)", len(d.Comments)))
		cs := slices.Clone(d.Comments)
		sort.SliceStable(cs, func(i, j int) bool { return cs[i].Created.Before(cs[j].Created) })
		for _, c := range cs {
			add(1, by(c), sDim)
			add(3, c.Body, sText)
		}
	}
	return out
}

// detailLines stacks the detail screen's sections: a header with the item's id, title, URL, author and its
// pipeline/draft state, then Fields with the note and every remaining field untruncated (the ci_jobs one job per
// line), keys in one column as wide as the longest key and wrapped values indented to the value column, then the
// fetched sections (detailSections).
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
		return append(out, m.detailSections(w)...)
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
	return append(out, m.detailSections(w)...)
}
