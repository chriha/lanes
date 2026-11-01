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
	"github.com/chriha/lanes/internal/trust"
	"github.com/chriha/lanes/internal/ui"
)

// version is set at build time: -ldflags "-X main.version=v1.0.0".
var version = "dev"

func main() {
	if msg := usageError(os.Args[1:]); msg != "" {
		fmt.Fprintln(os.Stderr, msg)
		os.Exit(2)
	}
	wd, _ := os.Getwd()
	os.Exit(run(os.Args[1:], env{
		wd: wd, isTTY: stdinIsTerminal(), stdout: os.Stdout, stderr: os.Stderr,
		ask:     ui.AskTrust,
		initApp: app.Init,
		hook:    session.RunHook,
		status:  runStatus,
		doctor:  func() int { return runDoctor(os.Stdout, realDoctorEnv()) },
		init: func() int {
			return runInit(os.Stdin, stdinIsTerminal(), os.Stdout, os.Stderr, wd, provider.OriginForge)
		},
		tui: runTUI,
	}))
}

func runTUI() int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", app.Name, err)
		return 1
	}
	if _, err := tea.NewProgram(ui.New(cfg), tea.WithAltScreen(), tea.WithReportFocus()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", app.Name, err)
		return 1
	}
	return 0
}

// env holds what run touches, so tests can see that nothing reads the project before it is trusted.
type env struct {
	wd             string
	isTTY          bool
	stdout, stderr io.Writer
	ask            func(root string) (yes, always bool, err error) // the trust dialog
	initApp        func(dir string)                                // app.Init: runs git in the project
	hook           func(event string)
	status         func()
	doctor         func() int
	init           func() int // writes .lanes.toml
	tui            func() int // loads the config and runs the TUI
}

// run dispatches args (os.Args[1:], already checked by usageError) and returns the exit code.
func run(args []string, e env) int {
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}
	if cmd == "--version" || cmd == "version" {
		fmt.Fprintln(e.stdout, app.Name, version)
		return 0
	}
	// Nothing below may run git, load the config or start a provider before the project is trusted.
	root := trust.Root(e.wd)
	hash, err := trust.HashRoot(root)
	trusted := false
	if err == nil {
		trusted, err = trust.IsTrusted(root, hash)
	}
	switch cmd {
	case "hook": // called by Claude inside sessions: never prompts, does nothing untrusted
		if trusted {
			e.hook(args[1])
		}
		return 0
	case "status": // tmux status bar: never prompts, prints nothing untrusted
		if trusted {
			e.initApp(e.wd)
			e.status()
		}
		return 0
	}
	if err != nil {
		fmt.Fprintf(e.stderr, "%s: %v\n", app.Name, err) // a corrupt trust.json trusts nothing
	}
	if !trusted {
		if !e.isTTY {
			fmt.Fprintf(e.stderr, "%s is not trusted; run %s in a terminal to trust it\n", root, app.Name)
			return 1
		}
		yes, _, err := e.ask(root)
		if err != nil {
			fmt.Fprintf(e.stderr, "%s: %v\n", app.Name, err)
			return 1
		}
		if !yes {
			fmt.Fprintf(e.stderr, "%s: not trusted\n", app.Name)
			return 1
		}
		if err := trust.Trust(root, hash); err != nil {
			fmt.Fprintf(e.stderr, "%s: %v\n", app.Name, err)
			return 1
		}
	}
	e.initApp(e.wd)
	switch cmd {
	case "doctor":
		return e.doctor()
	case "init":
		code := e.init()
		// the user wrote .lanes.toml through lanes: trust its new content
		if hash, err := trust.HashRoot(root); err != nil {
			fmt.Fprintf(e.stderr, "%s: %v\n", app.Name, err)
		} else if err := trust.Trust(root, hash); err != nil {
			fmt.Fprintf(e.stderr, "%s: %v\n", app.Name, err)
		}
		return code
	}
	return e.tui()
}

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
