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

var preferred = []string{"iid", "number", "key", "branch", "target", "ticket", "labels", "reviewers", "assignees", "blocked_by", "blocking", "related", "approved_by", "ci", "ci_jobs", "diverged", "open_threads"}

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

// labelsLine renders each label in its theme color, joined by ", " in the text color; it is empty when v is not a
// list or has no labels.
func labelsLine(v any) string {
	var labels []string
	switch x := v.(type) {
	case []string:
		labels = x
	case []any: // decoded from JSON (cache, command provider)
		for _, e := range x {
			labels = append(labels, fmt.Sprint(e))
		}
	}
	parts := make([]string, len(labels))
	for i, l := range labels {
		parts[i] = labelStyle(l).Render(l)
	}
	return strings.Join(parts, sText.Render(", "))
}

var relationKeys = map[string]bool{"blocked_by": true, "blocking": true, "related": true}

// relationLine renders a relation's open ids, then its closed ones (from closed_links, "<relation>:<id>") dimmed
// with a ✓ suffix; it is empty when there are none, or when rel is not a relation field.
func relationLine(it item.Item, rel string) string {
	if !relationKeys[rel] {
		return ""
	}
	var parts []string
	if v := fmtVal(it.Fields[rel]); v != "" {
		parts = append(parts, sText.Render(v))
	}
	var closed []string
	switch x := it.Fields["closed_links"].(type) {
	case []string:
		closed = x
	case []any: // decoded from JSON (cache, command provider)
		for _, e := range x {
			closed = append(closed, fmt.Sprint(e))
		}
	}
	for _, c := range closed {
		if id, ok := strings.CutPrefix(c, rel+":"); ok {
			parts = append(parts, sDim.Render(id+" ✓"))
		}
	}
	return strings.Join(parts, sText.Render(", "))
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
		jobs := -1 // index of the ci_jobs line, -1 when there is none
		for _, k := range keys {
			if k == "note" && cur.Fields[k] == any(note) {
				continue // shown above
			}
			if rel := relationLine(cur, k); rel != "" {
				lines = append(lines, " "+sDim.Render(fmt.Sprintf("%-12s", k))+rel)
				continue
			}
			if relationKeys[k] {
				continue // no open or closed ids
			}
			if k == "labels" {
				if l := labelsLine(cur.Fields[k]); l != "" {
					lines = append(lines, " "+sDim.Render(fmt.Sprintf("%-12s", k))+l)
					continue
				}
			}
			if v := fmtVal(cur.Fields[k]); v != "" && v != "[]" && v != "0" {
				if k == "ci_jobs" {
					jobs = len(lines)
				}
				lines = append(lines, " "+sDim.Render(fmt.Sprintf("%-12s", k))+sText.Render(v))
			}
		}
		// the session and last-run lines outrank the field lines: trim the fields to make room, but keep the
		// ci_jobs line (the failed jobs) as the last field line when it would be cut
		extra := m.previewSessions(cur)
		if room := previewH - len(extra); len(lines) > room {
			if jobs >= room && room > 0 {
				jobsLine := lines[jobs]
				lines = append(lines[:room-1], jobsLine)
			} else {
				lines = lines[:max(room, 0)]
			}
		}
		lines = append(lines, extra...)
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
		out = append(out, " "+sDim.Render(fmt.Sprintf("%-12s", "last run"))+sText.Render(lastRun(evs[n-1])))
	}
	return out
}

// lastRun is the preview's last-run text for an item's newest history event: action, duration, tokens and outcome
// joined by " · ", leaving out the parts the event lacks.
// Tokens count input, cache writes and output; cache reads are left out.
func lastRun(e history.Event) string {
	parts := []string{e.Action}
	if e.Dur > 0 {
		parts = append(parts, runDur(e.Dur))
	}
	if t := e.Tokens; t != nil {
		if n := t.Input + t.CacheWrite + t.Output; n > 0 {
			parts = append(parts, tokenCount(n)+" tok")
		}
	}
	switch {
	case e.Event == "finished":
		parts = append(parts, "✓")
	case e.Event == "failed" && e.Exit != 0:
		parts = append(parts, fmt.Sprintf("✗ %d", e.Exit))
	case e.Event == "failed":
		parts = append(parts, "✗")
	default:
		parts = append(parts, e.Event)
	}
	return strings.Join(parts, " · ")
}

// runDur is a session's or run's duration in seconds: 42s, 14m, 1h05m.
func runDur(sec int64) string {
	switch {
	case sec < 60:
		return fmt.Sprintf("%ds", sec)
	case sec < 3600:
		return fmt.Sprintf("%dm", sec/60)
	}
	return fmt.Sprintf("%dh%02dm", sec/3600, sec%3600/60)
}

// tokenCount is a compact token count, rounded down: 850, 12k, 2.1M.
func tokenCount(n int) string {
	short := func(n, unit int, suffix string) string {
		if n < 10*unit && n%unit >= unit/10 {
			return fmt.Sprintf("%d.%d%s", n/unit, n%unit/(unit/10), suffix)
		}
		return fmt.Sprintf("%d%s", n/unit, suffix)
	}
	switch {
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 1_000_000:
		return short(n, 1000, "k")
	}
	return short(n, 1_000_000, "M")
}
