package ui

import (
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/chriha/lanes/internal/provider"
)

// builtinTypes are the provider types whose field keys the help always lists ("auto" resolves to gitlab or github).
var builtinTypes = []string{"gitlab", "github", "github-issues", "linear"}

// fieldsHelp renders the help's fields section: the keys filters and action templates can use, per provider type,
// for every configured command provider (observed in its last fetched items) and for actions only.
func (m *Model) fieldsHelp() string {
	type row struct {
		label string
		keys  []string
		dim   string // shown instead of keys
	}
	rows := []row{{label: "common", keys: []string{"id", "title", "url", "author", "draft", "note"}}}
	for _, typ := range builtinTypes {
		rows = append(rows, row{label: typ, keys: provider.Fields(typ)})
	}
	names := make([]string, 0, len(m.cfg.Provider))
	for name, pc := range m.cfg.Provider {
		if pc.Type == "command" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	for _, name := range names {
		r := row{label: name + " (command)"}
		seen := map[string]bool{}
		for _, it := range m.data[name] {
			for k := range it.Fields {
				if !seen[k] {
					seen[k] = true
					r.keys = append(r.keys, k)
				}
			}
		}
		slices.Sort(r.keys)
		switch {
		case len(m.data[name]) == 0 && m.fetchedAt[name].IsZero():
			r.dim = "fetch pending"
		case len(r.keys) == 0:
			r.dim = "none"
		}
		rows = append(rows, r)
	}
	rows = append(rows, row{label: "actions only", keys: []string{"settings", "session", "action", "vars"}})

	labelW := 18
	for _, r := range rows {
		labelW = max(labelW, lipgloss.Width(r.label)+2)
	}
	indent := 2 + labelW
	var b strings.Builder
	b.WriteString("\n" + sTitle.Render(" fields") + sDim.Render("  (in filters, and as {{.name}} in action templates)") + "\n\n")
	for _, r := range rows {
		b.WriteString("  " + sKey.Render(r.label+strings.Repeat(" ", labelW-lipgloss.Width(r.label))))
		if r.dim != "" {
			b.WriteString(sDim.Render(r.dim) + "\n")
			continue
		}
		words := make([]string, len(r.keys))
		for i, k := range r.keys {
			words[i] = "." + k
		}
		for i, l := range wrapWords(words, max(m.width-indent, 20)) {
			if i > 0 {
				b.WriteString(strings.Repeat(" ", indent))
			}
			b.WriteString(sText.Render(l) + "\n")
		}
	}
	return b.String()
}

// wrapWords joins words with spaces into lines at most width wide; a word longer than width gets a line of its own.
func wrapWords(words []string, width int) []string {
	var lines []string
	cur := ""
	for _, w := range words {
		switch {
		case cur == "":
			cur = w
		case lipgloss.Width(cur)+1+lipgloss.Width(w) <= width:
			cur += " " + w
		default:
			lines = append(lines, cur)
			cur = w
		}
	}
	if cur != "" || len(lines) == 0 {
		lines = append(lines, cur)
	}
	return lines
}
