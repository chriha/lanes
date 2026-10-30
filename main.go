package main

import (
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/provider"
	"github.com/chriha/lanes/internal/session"
	"github.com/chriha/lanes/internal/ui"
)

// version is set at build time: -ldflags "-X main.version=v1.0.0".
var version = "dev"

func main() {
	if msg := usageError(os.Args[1:]); msg != "" {
		fmt.Fprintln(os.Stderr, msg)
		os.Exit(2)
	}
	if len(os.Args) == 3 && os.Args[1] == "hook" {
		session.RunHook(os.Args[2])
		return
	}
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Println(app.Name, version)
		return
	}
	wd, _ := os.Getwd()
	app.Init(wd)
	if len(os.Args) == 2 && os.Args[1] == "doctor" {
		os.Exit(runDoctor(os.Stdout, realDoctorEnv()))
	}
	if len(os.Args) == 2 && os.Args[1] == "init" {
		os.Exit(runInit(os.Stdin, stdinIsTerminal(), os.Stdout, os.Stderr, wd, provider.OriginForge))
	}
	if len(os.Args) == 2 && os.Args[1] == "status" {
		runStatus()
		return
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", app.Name, err)
		os.Exit(1)
	}
	if _, err := tea.NewProgram(ui.New(cfg), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", app.Name, err)
		os.Exit(1)
	}
}

// env holds what run touches, so tests can see that nothing reads the project before it is trusted.
type env struct {
	wd             string
	isTTY          bool
	stdout, stderr io.Writer
	projectPath    func() string                   // the .lanes.toml that config would load ("" = none)
	ask            func(root string) (bool, error) // the trust dialog
	initApp        func(dir string)                // app.Init: runs git in the project
	hook           func(event string)
	status         func()
	doctor         func() int
	init           func() int // writes .lanes.toml
	tui            func() int // loads the config and runs the TUI
}

// run dispatches args (os.Args[1:], already checked by usageError) and returns the exit code.
func run(args []string, e env) int { return 0 }

// runStatus prints the cwd project's sessions that need input, for tmux status bars and shell prompts.
// Errors (tmux missing, no server, state dir not writable) print nothing: a status bar must not show them.
func runStatus() {
	m, err := session.NewManager()
	if err != nil {
		return
	}
	infos, err := m.List()
	if err != nil {
		return
	}
	if line := session.StatusLine(infos); line != "" {
		fmt.Println(line)
	}
}

// stdinIsTerminal reports whether stdin is an interactive terminal (no new dependency: a character device).
func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	null, err := os.Stat(os.DevNull)
	return err != nil || !os.SameFile(fi, null) // /dev/null is a character device but not a terminal
}

// usageError returns a usage message when args (os.Args[1:]) misuse a subcommand, or "" when they are fine.
func usageError(args []string) string {
	if len(args) == 0 {
		return ""
	}
	want := map[string]int{"hook": 1, "doctor": 0, "init": 0, "status": 0, "version": 0, "--version": 0}
	n, ok := want[args[0]]
	if !ok || len(args)-1 == n {
		return ""
	}
	if args[0] == "hook" {
		return "usage: " + app.Name + " hook <event>"
	}
	return "usage: " + app.Name + " " + args[0]
}
