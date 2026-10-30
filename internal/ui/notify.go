package ui

import (
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

// notify raises a desktop notification; best effort, never blocks the UI.
func notify(title, body string) {
	switch runtime.GOOS {
	case "darwin":
		esc := func(s string) string { return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) }
		_ = exec.Command("osascript", "-e",
			`display notification "`+esc(body)+`" with title "`+esc(title)+`"`).Start()
	default:
		if p, err := exec.LookPath("notify-send"); err == nil {
			_ = exec.Command(p, title, body).Start()
		}
	}
}

// openItemURL opens an item's URL with m.open (the system opener when unset), but only if it is an
// http(s) URL with a host: provider data must not reach the opener as file:, custom schemes or options.
func (m *Model) openItemURL(raw string) {
	u, err := url.Parse(raw)
	if err != nil || (!strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https")) || u.Host == "" {
		m.notice = "not opened: only http(s) URLs can be opened"
		return
	}
	open := m.open
	if open == nil {
		open = openURL
	}
	if err := open(raw); err != nil {
		m.notice = "open failed: " + err.Error()
	}
}

// openURL opens a URL in the default browser.
func openURL(u string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	return exec.Command(opener, u).Start()
}
