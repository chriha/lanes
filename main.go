package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/session"
	"github.com/chriha/lanes/internal/ui"
)

// version is set at build time: -ldflags "-X main.version=v1.0.0".
var version = "dev"

func main() {
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
