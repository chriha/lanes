package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/config"
)

// runInit writes a commented .lanes.toml to dir and returns the exit code; detect reports the origin's forge.
// An existing file is never overwritten.
func runInit(stdout, stderr io.Writer, dir string, detect func() (typ, repo string, err error)) int {
	typ, repo, err := detect()
	if err != nil {
		typ, repo = "", "" // no origin: the template comments the provider out and says why
	}
	path, err := config.InitProject(dir, typ, repo)
	if errors.Is(err, fs.ErrExist) {
		fmt.Fprintf(stderr, "%s: %s already exists\n", app.Name, config.ProjectFile)
		return 1
	}
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", app.Name, err)
		return 1
	}
	fmt.Fprintln(stdout, path)
	return 0
}
