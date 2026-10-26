package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/filter"
)

type editorDone struct{}

func (m *Model) runCommand(line string) (tea.Model, tea.Cmd) {
	name, arg, _ := strings.Cut(line, " ")
	arg = strings.TrimSpace(arg)
	switch name {
	case "":
	case "q", "quit":
		return m, tea.Quit
	case "refresh", "r":
		m.notice = "refreshing…"
		return m, m.refreshAll()
	case "reload":
		return m, m.reload()
	case "config":
		m.notice = "global: " + config.Path()
		if pp := config.ProjectPath(); pp != "" {
			m.notice += " · project: " + pp
		}
	case "edit":
		editor := os.Getenv("VISUAL")
		if editor == "" {
			editor = os.Getenv("EDITOR")
		}
		if editor == "" {
			editor = "vi"
		}
		path := config.Path()
		if arg == "project" {
			if path = config.ProjectPath(); path == "" {
				wd, _ := os.Getwd()
				path = filepath.Join(wd, config.ProjectFile)
				_ = os.WriteFile(path, []byte("# project config: overlays the global config (README: Project config)\n"), 0o644)
			}
		}
		cmd := exec.Command("/bin/sh", "-c", editor+" "+shq(path))
		return m, tea.ExecProcess(cmd, func(error) tea.Msg { return editorDone{} })
	case "view", "v":
		for i, v := range m.views {
			if v.cfg.Name == arg {
				m.switchView(i)
				return m, nil
			}
		}
		names := make([]string, len(m.views))
		for i, v := range m.views {
			names[i] = v.cfg.Name
		}
		sort.Strings(names)
		m.notice = fmt.Sprintf("unknown view %q (have: %s)", arg, strings.Join(names, ", "))
	case "filter":
		m.setFilter(arg)
	case "newview":
		m.newView(arg)
	case "delview":
		m.delView(arg)
	case "sessions":
		m.openSessions()
	case "run":
		act, ok := m.cfg.Action[arg]
		if !ok {
			m.notice = fmt.Sprintf("unknown action %q", arg)
			return m, nil
		}
		return m, m.runAction(arg, act)
	default:
		m.notice = "unknown command: " + name
	}
	return m, nil
}

func (m *Model) inConfig(name string) bool {
	for _, v := range m.cfg.View {
		if v.Name == name {
			return true
		}
	}
	return false
}

func (m *Model) setFilter(arg string) {
	v := m.views[m.cur]
	name := v.cfg.Name
	switch arg {
	case "":
		m.notice = "filter: " + v.cfg.Filter
		return
	case "reset":
		if !m.inConfig(name) {
			m.notice = "only config views can be reset"
			return
		}
		delete(m.ov.Filters, name)
	default:
		if _, err := filter.Compile(arg); err != nil {
			m.notice = "filter: " + err.Error()
			return
		}
		if m.inConfig(name) {
			m.ov.Filters[name] = arg
		} else {
			for i := range m.ov.Views {
				if m.ov.Views[i].Name == name {
					m.ov.Views[i].Filter = arg
				}
			}
		}
	}
	m.saveViews("filter updated")
}

func (m *Model) newView(arg string) {
	name, expr, _ := strings.Cut(arg, " ")
	expr = strings.TrimSpace(expr)
	for _, v := range m.views {
		if name == "" || v.cfg.Name == name {
			m.notice = "newview needs a new, unique name: :newview <name> <expr>"
			return
		}
	}
	if _, err := filter.Compile(expr); err != nil {
		m.notice = "filter: " + err.Error()
		return
	}
	m.ov.Views = append(m.ov.Views, config.View{Name: name, Provider: m.views[m.cur].cfg.Provider, Filter: expr})
	m.saveViews("view " + name + " created")
	for i, v := range m.views {
		if v.cfg.Name == name {
			m.switchView(i)
		}
	}
}

func (m *Model) delView(name string) {
	for i, v := range m.ov.Views {
		if v.Name == name {
			m.ov.Views = append(m.ov.Views[:i], m.ov.Views[i+1:]...)
			m.saveViews("view " + name + " deleted")
			return
		}
	}
	m.notice = fmt.Sprintf("%q is not a view created with :newview (config views: edit the config)", name)
}

func (m *Model) saveViews(ok string) {
	if err := m.ov.Save(); err != nil {
		m.notice = "saving views: " + err.Error()
		return
	}
	m.applyConfig(m.cfg)
	m.notice = ok
}
