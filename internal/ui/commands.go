package ui

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/filter"
	"github.com/chriha/lanes/internal/provider"
)

// editorDone ends :edit; project is set for :edit project, whose file the user wrote through lanes.
type editorDone struct{ project bool }

func (m *Model) runCommand(line string) (tea.Model, tea.Cmd) {
	name, arg, _ := strings.Cut(line, " ")
	arg = strings.TrimSpace(arg)
	switch name {
	case "":
	case "q", "quit":
		return m, tea.Quit
	case "refresh", "r":
		m.say(lvInfo, "refreshing…")
		return m, m.refreshAll()
	case "reload":
		return m, m.reload()
	case "config":
		m.say(lvInfo, "global: "+config.Path())
		if pp := config.ProjectPath(); pp != "" {
			m.say(lvInfo, m.notice+" · project: "+pp)
		}
	case "edit":
		path := config.Path()
		if arg == "project" {
			if path = config.ProjectPath(); path == "" {
				detect := m.originForge
				if detect == nil {
					detect = provider.OriginForge
				}
				typ, repo, _ := detect() // no origin: the template comments the provider out
				wd, _ := os.Getwd()
				var err error
				if path, err = config.InitProject(wd, typ, repo); err != nil && !errors.Is(err, fs.ErrExist) {
					m.say(lvErr, err.Error()) // nothing to open
					return m, nil
				}
			}
		}
		cmd := exec.Command("/bin/sh", "-c", editor()+" "+shq(path))
		return m, tea.ExecProcess(cmd, func(error) tea.Msg { return editorDone{project: arg == "project"} })
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
		m.say(lvWarn, fmt.Sprintf("unknown view %q (have: %s)", arg, strings.Join(names, ", ")))
	case "filter":
		return m, m.setFilter(arg)
	case "newview":
		m.newView(arg)
	case "delview":
		m.delView(arg)
	case "sessions":
		m.openSessions()
	case "prune":
		if arg != "" && arg != "failed" {
			m.say(lvWarn, "usage: :prune [failed]")
			return m, nil
		}
		if m.sess == nil {
			m.say(lvWarn, "tmux unavailable")
			return m, nil
		}
		mgr, failedOnly := m.sess, arg == "failed"
		return m, func() tea.Msg {
			n, err := mgr.Prune(failedOnly)
			d := doneMsg{notice: pruneNotice(n, err)}
			if err != nil {
				d.level = lvErr
			}
			return d
		}
	case "note":
		if it := m.items(); m.cursor < len(it) {
			m.setNote(it[m.cursor].ID, arg)
		} else {
			m.say(lvWarn, "no item to note")
		}
	case "snooze":
		m.snoozeTargets(arg)
	case "snoozed":
		m.showSnoozed = !m.showSnoozed
		m.say(lvInfo, "hiding snoozed items")
		if m.showSnoozed {
			m.say(lvInfo, "showing snoozed items")
		}
		m.rebuild()
	case "resume":
		if it := m.items(); m.cursor < len(it) {
			return m, m.resume(it[m.cursor])
		}
		m.say(lvWarn, "no item to resume")
	case "run":
		act, ok := m.cfg.Action[arg]
		if !ok {
			m.say(lvWarn, fmt.Sprintf("unknown action %q", arg))
			return m, nil
		}
		return m, m.runAction(arg, act)
	default:
		m.say(lvWarn, "unknown command: "+name)
	}
	return m, nil
}

// pruneNotice is the footer after :prune: "pruned N sessions" ("pruned 1 session" for one), with the first
// failed kill appended.
func pruneNotice(n int, err error) string {
	s := fmt.Sprintf("pruned %d sessions", n)
	if n == 1 {
		s = "pruned 1 session"
	}
	if err != nil {
		s += "; " + err.Error()
	}
	return s
}

// editor is the user's editor command: $VISUAL, then $EDITOR, then vi.
func editor() string {
	if e := os.Getenv("VISUAL"); e != "" {
		return e
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "vi"
}

func (m *Model) inConfig(name string) bool {
	for _, v := range m.cfg.View {
		if v.Name == name {
			return true
		}
	}
	return false
}

func (m *Model) setFilter(arg string) tea.Cmd {
	v := m.views[m.cur]
	name := v.cfg.Name
	switch arg {
	case "":
		m.say(lvInfo, "filter: "+v.cfg.Filter)
		return nil
	case "reset":
		if !m.inConfig(name) {
			m.say(lvWarn, "only config views can be reset")
			return nil
		}
		delete(m.ov.Filters, name)
	case "save":
		return m.saveFilter(name)
	default:
		if _, err := filter.Compile(arg); err != nil {
			m.say(lvErr, "filter: "+err.Error())
			return nil
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
	return nil
}

// saveFilter writes the view's in-app filter edit into the config file defining the view, drops the
// override and reloads the config (like ctrl-r).
func (m *Model) saveFilter(name string) tea.Cmd {
	if !m.inConfig(name) {
		m.say(lvWarn, "only config views can be saved")
		return nil
	}
	expr, ok := m.ov.Filters[name]
	if !ok {
		m.say(lvWarn, "filter: nothing to save")
		return nil
	}
	path, err := config.ViewFile(name)
	if err == nil {
		err = config.SaveViewFilter(path, name, expr)
	}
	if errors.Is(err, config.ErrNotSavable) {
		m.say(lvErr, "filter: can't save, edit "+path+" by hand")
		return nil
	}
	if err != nil {
		m.say(lvErr, "filter: "+err.Error())
		return nil
	}
	delete(m.ov.Filters, name) // before the reload, so the view does not show the old * state
	saved := "filter saved to " + path
	if err := m.ov.Save(); err != nil { // the file already holds the filter: still reload
		saved += "; saving views: " + err.Error()
	}
	cfg, err := config.Load()
	if err != nil {
		m.say(lvErr, saved+"; config: "+err.Error())
		return nil // the tick's reload reports it again
	}
	m.applyConfig(cfg)
	m.say(lvInfo, saved)
	return m.refreshAll()
}

func (m *Model) newView(arg string) {
	name, expr, _ := strings.Cut(arg, " ")
	expr = strings.TrimSpace(expr)
	for _, v := range m.views {
		if name == "" || v.cfg.Name == name {
			m.say(lvWarn, "newview needs a new, unique name: :newview <name> <expr>")
			return
		}
	}
	if _, err := filter.Compile(expr); err != nil {
		m.say(lvErr, "filter: "+err.Error())
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
	m.say(lvWarn, fmt.Sprintf("%q is not a view created with :newview (config views: edit the config)", name))
}

func (m *Model) saveViews(ok string) {
	if err := m.ov.Save(); err != nil {
		m.say(lvErr, "saving views: "+err.Error())
		return
	}
	m.applyConfig(m.cfg)
	if bak := m.ov.BackedUp(); bak != "" {
		ok += "; the unreadable views.json was kept as " + bak
	}
	m.say(lvInfo, ok)
}
