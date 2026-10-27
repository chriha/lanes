package main

import "io"

// runInit writes a commented .lanes.toml to dir and returns the exit code; detect reports the origin's forge.
func runInit(stdout, stderr io.Writer, dir string, detect func() (typ, repo string, err error)) int {
	return 0
}
