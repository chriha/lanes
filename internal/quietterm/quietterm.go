// Package quietterm keeps bubbletea's package init from querying the terminal.
//
// bubbletea's init calls lipgloss.HasDarkBackground, which sends OSC 11 and waits up to 5 s for an answer from a
// terminal that may never give one. termenv skips that query when TERM is screen, tmux or dumb, so this package
// sets TERM=dumb in its own init and main restores it first thing with Restore. The package imports nothing outside
// the standard library: that makes it ready from the start, and Go initialises ready packages in import path
// order, so it runs before termenv, lipgloss and bubbletea, which all sort after it or wait on packages that do.
// TestStartupDoesNotWaitForTheColourQuery in the main package fails if that ever stops holding.
package quietterm

import "os"

var (
	origTerm string
	hadTerm  bool
)

func init() { mute() }

// mute hides TERM from termenv until Restore.
func mute() {
	origTerm, hadTerm = os.LookupEnv("TERM")
	set("TERM", "dumb")
}

// Restore puts TERM back as the process was started with.
func Restore() {
	if hadTerm {
		set("TERM", origTerm)
	} else {
		if err := os.Unsetenv("TERM"); err != nil {
			panic(err)
		}
	}
}

// set panics on an error: Setenv fails only for an invalid name, and TERM is not one.
func set(key, value string) {
	if err := os.Setenv(key, value); err != nil {
		panic(err)
	}
}
