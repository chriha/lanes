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

// copyIDs copies the IDs of the marked items (else the cursor item), space-separated, and sets the notice.
func (m *Model) copyIDs() {
	var ids []string
	for _, it := range m.targets() {
		ids = append(ids, it.ID)
	}
	if len(ids) == 0 {
		return
	}
	cp := m.clip
	if cp == nil {
		cp = systemCopy
	}
	if err := cp(strings.Join(ids, " ")); err != nil {
		m.notice = "copy: " + err.Error()
		return
	}
	m.notice = "copied " + ids[0]
	if len(ids) > 1 {
		m.notice = fmt.Sprintf("copied %d IDs", len(ids))
	}
}
