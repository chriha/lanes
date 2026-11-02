package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// A terminal that never answers the colour query must not hold up the process before main: bubbletea's package init
// asks for the background and, unless something settled it earlier, waits termenv's 5 s for the answer.
func TestStartupDoesNotWaitForTheColourQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	script, err := exec.LookPath("script")
	if err != nil {
		t.Skip("script(1) not available to provide a terminal")
	}
	bin := filepath.Join(t.TempDir(), "lanes")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	// script gives the child a terminal on stdout that, unlike a real one, never answers OSC 11.
	args := []string{"-q", "/dev/null", bin, "version"}
	if runtime.GOOS == "linux" {
		args = []string{"-q", "-e", "-c", bin + " version", "/dev/null"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, script, args...)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORFGBG=", "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
	cmd.Stdin = nil
	start := time.Now()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("startup took %v with a silent terminal, want under 1s", d)
	}
}
