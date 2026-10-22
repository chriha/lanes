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
