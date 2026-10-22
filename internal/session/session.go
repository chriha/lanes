// Package session runs actions in detached tmux sessions on a dedicated tmux server
// (tmux -L <app name>), so it never touches the user's own tmux.
package session

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/item"
)

// Info is one tmux session started by an action. The session name encodes action and item: <action>~<item>.
type Info struct {
	Name    string
	Action  string
	ItemKey string // Key(item ID)
	Dead    bool
	Exit    int
	Created time.Time
	Claude  string // working | asking | waiting, empty if the command does not report status
}

var unsafe = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// Key is the tmux-safe form of an item ID.
func Key(itemID string) string { return unsafe.ReplaceAllString(itemID, "_") }

func Name(action, itemID string) string { return action + "~" + Key(itemID) }

type Manager struct{ conf string }

func NewManager() (*Manager, error) {
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil, errors.New("tmux not found in PATH")
	}
	dir := app.StateDir()
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o755); err != nil {
		return nil, err
	}
	// exit-empty off keeps the server (and thus remain-on-exit) alive between sessions.
	conf := filepath.Join(dir, "tmux.conf")
	body := "set -g remain-on-exit on\nset -g exit-empty off\nset -g status-right ' detach: C-b d '\nset -g mouse on\n"
	if err := os.WriteFile(conf, []byte(body), 0o644); err != nil {
		return nil, err
	}
	return &Manager{conf: conf}, nil
}

func (m *Manager) tmux(args ...string) *exec.Cmd {
	return exec.Command("tmux", append([]string{"-L", app.Socket(), "-f", m.conf}, args...)...)
}

func (m *Manager) List() ([]Info, error) {
	const sep = "\t"
	out, err := m.tmux("list-panes", "-a", "-F",
		strings.Join([]string{"#{session_name}", "#{pane_dead}", "#{pane_dead_status}", "#{session_created}"}, sep)).Output()
	if err != nil {
		return nil, nil // no server running = no sessions
	}
	var infos []Info
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Split(line, sep)
		if len(f) != 4 {
			continue
		}
		action, key, ok := strings.Cut(f[0], "~")
		if !ok {
			continue
		}
		created, _ := strconv.ParseInt(f[3], 10, 64)
		exit, _ := strconv.Atoi(f[2])
		in := Info{Name: f[0], Action: action, ItemKey: key, Dead: f[1] == "1", Exit: exit, Created: time.Unix(created, 0)}
		if !in.Dead {
			in.Claude = readState(in.Name)
		}
		infos = append(infos, in)
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Created.After(infos[j].Created) })
	return infos, nil
}

// Start launches command in the user's shell inside a new detached session.
func (m *Manager) Start(name, cwd, command string) error {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	_ = os.Remove(statePath(name))
	script := fmt.Sprintf("export LANES_SESSION=%s LANES_STATE_DIR=%s; %s", shq(name), shq(app.StateDir()), command)
	args := []string{"new-session", "-d", "-s", name, "-x", "220", "-y", "55"}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	args = append(args, "--", shell, "-ic", script)
	if out, err := m.tmux(args...).CombinedOutput(); err != nil {
		return fmt.Errorf("tmux: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (m *Manager) Kill(name string) error {
	_ = os.Remove(statePath(name))
	return m.tmux("kill-session", "-t", "="+name).Run()
}

// JumpCmd returns a command that attaches to the session: in a popup when the user is inside tmux,
// otherwise a plain attach (the caller suspends the TUI around it).
func (m *Manager) JumpCmd(name string) (cmd *exec.Cmd, popup bool) {
	attach := fmt.Sprintf("env -u TMUX tmux -L %s -f %s attach -t %s", app.Socket(), shq(m.conf), shq("="+name))
	if os.Getenv("TMUX") != "" {
		return exec.Command("tmux", "display-popup", "-E", "-w", "92%", "-h", "92%", attach), true
	}
	return m.tmux("attach", "-t", "="+name), false
}

func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Badge renders the session as a row badge.
func (i Info) Badge() item.Badge {
	switch {
	case i.Dead && i.Exit == 0:
		return item.Badge{Text: "✓ " + i.Action, Kind: "ok"}
	case i.Dead:
		return item.Badge{Text: fmt.Sprintf("✗ %s (%d)", i.Action, i.Exit), Kind: "bad"}
	case i.Claude == "working":
		return item.Badge{Text: "◐ " + i.Action + ": working", Kind: "info"}
	case i.Claude == "asking":
		return item.Badge{Text: "? " + i.Action + ": asking", Kind: "warn"}
	case i.Claude == "waiting":
		return item.Badge{Text: "● " + i.Action + ": waiting", Kind: "warn"}
	}
	return item.Badge{Text: "▶ " + i.Action, Kind: "info"}
}
