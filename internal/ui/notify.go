package ui

import (
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

// openItemURL opens an item's URL with m.open, or the system opener when unset.
func (m *Model) openItemURL(u string) {
	if m.open != nil {
		_ = m.open(u)
		return
	}
	openURL(u)
}

// openURL opens a URL in the default browser.
func openURL(url string) {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	_ = exec.Command(opener, url).Start()
}
