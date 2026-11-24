package ui

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

var goosName = runtime.GOOS

// clipTools lists the system clipboard commands to try, per GOOS, in order.
func clipTools(goos string) [][]string {
	if goos == "darwin" {
		return [][]string{{"pbcopy"}}
	}
	return [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "-b"}}
}

// copyText puts text on the clipboard: the first system tool found, else an OSC 52 sequence to osc.
func copyText(goos string, look func(string) (string, error), run func(path string, args []string, text string) error, osc io.Writer, text string) error {
	for _, t := range clipTools(goos) {
		if p, err := look(t[0]); err == nil {
			return run(p, t[1:], text)
		}
	}
	_, err := fmt.Fprintf(osc, "\x1b]52;c;%s\a", base64.StdEncoding.EncodeToString([]byte(text)))
	return err
}

func runClipTool(path string, args []string, text string) error {
	c := exec.Command(path, args...)
	c.Stdin = strings.NewReader(text)
	return c.Run()
}

func systemCopy(text string) error {
	return copyText(goosName, exec.LookPath, runClipTool, os.Stdout, text)
}

// copyOut puts text on the clipboard (the m.clip hook in tests) and says so as "copied <first>", plus "copied N
// <noun>" for several; a failure becomes a "copy: <err>" error notice.
func (m *Model) copyOut(text, first string, n int, noun string) {
	cp := m.clip
	if cp == nil {
		cp = systemCopy
	}
	if err := cp(text); err != nil {
		m.say(lvErr, "copy: "+err.Error())
		return
	}
	m.say(lvInfo, "copied "+first)
	if n > 1 {
		m.say(lvInfo, fmt.Sprintf("copied %d %s", n, noun))
	}
}

// copyIDs copies the IDs of the marked items (else the cursor item), space-separated, and sets the notice.
func (m *Model) copyIDs() {
	var ids []string
	for _, it := range m.targets() {
		ids = append(ids, it.ID)
	}
	if len(ids) == 0 {
		return
	}
	m.copyOut(strings.Join(ids, " "), ids[0], len(ids), "IDs")
}

// copyURLs copies the URLs of the marked items (else the cursor item), one per line, skipping items without one.
func (m *Model) copyURLs() {
	var urls []string
	for _, it := range m.targets() {
		if it.URL != "" {
			urls = append(urls, it.URL)
		}
	}
	m.copyURLList(urls)
}

// copyURLList copies urls newline-separated; with none it warns and leaves the clipboard alone.
func (m *Model) copyURLList(urls []string) {
	if len(urls) == 0 {
		m.say(lvWarn, "nothing to copy: no URL")
		return
	}
	m.copyOut(strings.Join(urls, "\n"), urls[0], len(urls), "URLs")
}
