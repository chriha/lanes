package ui

import "github.com/chriha/lanes/internal/session"

// sessionRow is one line of the sessions screen.
type sessionRow struct {
	info   session.Info
	label  string // item ID + title, or the session's item key when no provider has the item
	orphan bool   // the item is in no view
}

// sessionRows builds the sessions screen rows in the order of infos (newest first).
func sessionRows(infos []session.Info, labelOf func(key string) string, inView map[string]bool) []sessionRow {
	return nil
}
