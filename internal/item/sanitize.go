package item

import (
	"strings"
	"unicode/utf8"
)

// Sanitize returns the items with terminal control characters removed from every string they carry (ID, title,
// URL, author, badges, pending names and all string values in Fields, recursively). Provider data is untrusted:
// an escape sequence in it could redraw the screen, set the clipboard (OSC 52) or plant hyperlinks (OSC 8).
// Tabs and newlines become a single space; C0 and C1 controls and DEL are dropped; other text is left alone.
func Sanitize(items []Item) []Item {
	if items == nil {
		return nil
	}
	out := make([]Item, len(items))
	for n, it := range items {
		it.ID, it.Title, it.URL, it.Author = clean(it.ID), clean(it.Title), clean(it.URL), clean(it.Author)
		if it.Badges != nil {
			it.Badges = make([]Badge, len(items[n].Badges))
			for i, b := range items[n].Badges {
				it.Badges[i] = Badge{Text: clean(b.Text), Kind: clean(b.Kind)}
			}
		}
		if it.Pending != nil {
			it.Pending = cleanStrings(it.Pending)
		}
		if it.Fields != nil {
			it.Fields = cleanMap(it.Fields)
		}
		out[n] = it
	}
	return out
}

// clean drops control characters; a run of tabs/newlines/carriage returns becomes one space.
func clean(s string) string {
	if utf8.ValidString(s) && !strings.ContainsFunc(s, isControl) {
		return s
	}
	var b strings.Builder
	space := false
	for i, r := range s {
		if r == utf8.RuneError {
			if _, w := utf8.DecodeRuneInString(s[i:]); w == 1 {
				continue // a stray byte, e.g. a raw 0x9b (8-bit CSI), is never text
			}
		}
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			if !space {
				b.WriteByte(' ')
			}
			space = true
			continue
		case isControl(r):
			continue
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}

func isControl(r rune) bool { return r < 0x20 || (r >= 0x7f && r <= 0x9f) }

func cleanStrings(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = clean(s)
	}
	return out
}

func cleanMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[clean(k)] = cleanValue(v)
	}
	return out
}

func cleanValue(v any) any {
	switch v := v.(type) {
	case string:
		return clean(v)
	case []string:
		return cleanStrings(v)
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = cleanValue(e)
		}
		return out
	case map[string]any:
		return cleanMap(v)
	default:
		return v
	}
}
