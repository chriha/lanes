package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/provider"
	"github.com/chriha/lanes/internal/session"
	"github.com/chriha/lanes/internal/trust"
	"github.com/chriha/lanes/internal/ui"
	"github.com/chriha/lanes/internal/update"
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
		version:   version,
		fetch:     update.GitHub(),
		upgrade:   upgradeCommand,
		updateOff: updateOff,
		ask:       ui.AskTrust,
		initApp:   app.Init,
		hook:      session.RunHook,
		status:    runStatus,
		doctor:    func() int { return runDoctor(os.Stdout, realDoctorEnv()) },
		init: func() int {
			return runInit(os.Stdin, stdinIsTerminal(), os.Stdout, os.Stderr, wd, provider.OriginForge)
		},
		tui: runTUI,
	}))
}

func runTUI(updates update.Checker, upgrade string) int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", app.Name, err)
		return 1
	}
	m := ui.New(cfg, ui.WithUpdateCheck(updates, upgrade))
	if _, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithReportFocus()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", app.Name, err)
		return 1
	}
	return 0
}

// upgradeCommand is the upgrade command for how the running binary was installed.
func upgradeCommand() string {
	exe, err := os.Executable()
	if err != nil {
		exe = "" // Hint falls back to the build info and then the releases page
	} else if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved // a Homebrew binary is a symlink into its Cellar
	}
	bi, _ := debug.ReadBuildInfo() // nil without build info
	return update.Hint(exe, bi)
}

// updateOff reports whether the config turns the release check off; a config that doesn't load turns it off too,
// so `lanes version` stays quiet about it.
func updateOff() bool {
	cfg, err := config.Load()
	return err != nil || cfg.Settings.NoUpdateCheck
}

// env holds what run touches, so tests can see that nothing reads the project before it is trusted.
type env struct {
	wd             string
	isTTY          bool
	stdout, stderr io.Writer
	version        string                                          // the running release, e.g. "v1.2.0"; "dev" never checks
	fetch          func(ctx context.Context) (string, error)       // asks GitHub for the newest release tag
	upgrade        func() string                                   // the upgrade command for how lanes was installed
	updateOff      func() bool                                     // settings.no_update_check (loads the config)
	ask            func(root string) (yes, always bool, err error) // the trust dialog
	initApp        func(dir string)                                // app.Init: runs git in the project
	hook           func(event string)
	status         func()
	doctor         func() int
	init           func() int                                       // writes .lanes.toml
	tui            func(updates update.Checker, upgrade string) int // loads the config and runs the TUI
}

// updates is the release checker of the current project; call it after initApp, which scopes the cache dir.
func (e env) updates() update.Checker {
	return update.Checker{
		Current:   e.version,
		Fetch:     e.fetch,
		Now:       time.Now,
		CachePath: filepath.Join(app.CacheDir(), "update.json"),
	}
}

// run dispatches args (os.Args[1:], already checked by usageError) and returns the exit code.
func run(args []string, e env) int {
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}
	if cmd == "--untrust" { // lowering trust is safe: no terminal needed, and nothing of the project runs
		return untrust(e)
	}
	// Nothing below may run git, load the config or start a provider before the project is trusted.
	root := trust.Root(e.wd)
	hash, err := trust.HashRoot(root)
	trusted := false
	if err == nil {
		trusted, err = trust.IsTrusted(root, hash)
	}
	switch cmd {
	case "version", "--version": // never prompts; the update hint reads the project's cache, so only when trusted
		fmt.Fprintln(e.stdout, app.Name, e.version)
		if trusted {
			e.initApp(e.wd)
			printUpdateHint(e)
		}
		return 0
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
		yes, always, err := e.ask(root)
		if err != nil {
			fmt.Fprintf(e.stderr, "%s: %v\n", app.Name, err)
			return 1
		}
		if !yes {
			fmt.Fprintf(e.stderr, "%s: not trusted\n", app.Name)
			return 1
		}
		if always {
			err = trust.TrustAlways(root)
		} else {
			err = trust.Trust(root, hash)
		}
		if err != nil {
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
	return e.tui(e.updates(), e.upgrade())
}

// printUpdateHint prints the newer release the last check found, from the cache only: it never asks the network.
func printUpdateHint(e env) {
	if e.updateOff() {
		return
	}
	if tag, newer := e.updates().Cached(); newer {
		fmt.Fprintf(e.stdout, "%s available · %s\n", tag, e.upgrade())
	}
}

// untrust removes the cwd root's trust entry, hash or always, so the next start asks again.
func untrust(e env) int {
	root := trust.Root(e.wd)
	removed, err := trust.Untrust(root)
	if err != nil {
		fmt.Fprintf(e.stderr, "%s: %v\n", app.Name, err)
		return 1
	}
	if removed {
		fmt.Fprintf(e.stdout, "%s is no longer trusted\n", root)
	} else {
		fmt.Fprintf(e.stdout, "%s was not trusted\n", root)
	}
	return 0
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
	want := map[string]int{"hook": 1, "doctor": 0, "init": 0, "status": 0, "version": 0, "--version": 0, "--untrust": 0}
	n, ok := want[args[0]]
	if !ok || len(args)-1 == n {
		return ""
	}
	if args[0] == "hook" {
		return "usage: " + app.Name + " hook <event>"
	}
	return "usage: " + app.Name + " " + args[0]
}
