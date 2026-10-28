package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/config"
)

// runInit writes a commented .lanes.toml to dir and returns the exit code; detect reports the origin's forge.
// An existing file is only overwritten after the user confirms on a terminal (isTTY); prompts go to stderr
// and are read from stdin, stdout carries only the written path.
func runInit(stdin io.Reader, isTTY bool, stdout, stderr io.Writer, dir string, detect func() (typ, repo string, err error)) int {
	typ, repo, err := detect()
	if err != nil {
		typ, repo = "", "" // no origin: the template comments the provider out and says why
	}
	path, err := config.InitProject(dir, typ, repo)
	if errors.Is(err, fs.ErrExist) {
		if !isTTY {
			fmt.Fprintf(stderr, "%s: %s already exists\n", app.Name, config.ProjectFile)
			return 1
		}
		in := bufio.NewReader(stdin)
		if !ask(in, stderr, config.ProjectFile+" already exists. Overwrite it with the default template? [y/N] ", false) {
			return 0
		}
		backup := ask(in, stderr, "Keep a backup of the current file as "+config.ProjectFile+".bak? [Y/n] ", true)
		path, err = config.OverwriteProject(dir, typ, repo, backup)
		if err == nil && backup {
			fmt.Fprintf(stderr, "backed up to %s.bak\n", path)
		}
	}
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", app.Name, err)
		return 1
	}
	fmt.Fprintln(stdout, path)
	return 0
}

// ask prints prompt and reads one line; def is the answer for Enter, EOF and (when def is false) anything but
// y/yes, (when def is true) anything but n/no.
func ask(in *bufio.Reader, w io.Writer, prompt string, def bool) bool {
	fmt.Fprint(w, prompt)
	line, _ := in.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	}
	return def
}
