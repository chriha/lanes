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
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
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

type Manager struct {
	conf string
	// run executes tmux with args and returns stdout; on failure the returned bytes are tmux's stderr.
	// Tests replace it so no real tmux server is contacted.
	run func(args ...string) ([]byte, error)
	// ps lists every process as "pid ppid pgid" lines; signal sends sig to the process group pgid.
	// Tests replace both so no real process is looked up or signalled.
	ps     func() ([]byte, error)
	signal func(pgid int, sig syscall.Signal) error
}

func NewManager() (*Manager, error) {
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil, errors.New("tmux not found in PATH")
	}
	dir := app.StateDir()
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), app.DirMode); err != nil {
		return nil, err
	}
	// exit-empty off keeps the server (and thus remain-on-exit) alive between sessions.
	conf := filepath.Join(dir, "tmux.conf")
	body := "set -g remain-on-exit on\nset -g exit-empty off\nset -g status-right ' detach: C-b d '\nset -g mouse on\n"
	if err := app.WriteFile(conf, []byte(body)); err != nil {
		return nil, err
	}
	m := &Manager{
		conf: conf,
		ps:   func() ([]byte, error) { return exec.Command("ps", "-A", "-o", "pid=,ppid=,pgid=").Output() },
		signal: func(pgid int, sig syscall.Signal) error {
			return syscall.Kill(-pgid, sig)
		},
	}
	m.run = func(args ...string) ([]byte, error) {
		out, err := m.tmux(args...).Output()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.Stderr, err
		}
		return out, err
	}
	return m, nil
}

func (m *Manager) tmux(args ...string) *exec.Cmd {
	return exec.Command("tmux", append([]string{"-L", app.Socket(), "-f", m.conf}, args...)...)
}

func (m *Manager) List() ([]Info, error) {
	const sep = "\t"
	out, err := m.run("list-panes", "-a", "-F",
		strings.Join([]string{"#{session_name}", "#{pane_dead}", deadStatus, "#{session_created}", "#{pane_current_command}"}, sep))
	if err != nil {
		return nil, nil // no server running = no sessions
	}
	var infos []Info
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Split(line, sep)
		if len(f) != 5 {
			continue
		}
		action, key, ok := strings.Cut(f[0], "~")
		if !ok {
			continue
		}
		created, _ := strconv.ParseInt(f[3], 10, 64)
		exit := exitCode(f[2])
		in := Info{Name: f[0], Action: action, ItemKey: key, Dead: f[1] == "1", Exit: exit, Created: time.Unix(created, 0)}
		if !in.Dead && !isShell(f[4]) {
			in.Claude = readState(in.Name)
		}
		infos = append(infos, in)
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Created.After(infos[j].Created) })
	return infos, nil
}

// deadStatus is a dead pane's exit status, or "sig<signal>" when a signal ended it (tmux leaves the status
// empty then).
const deadStatus = "#{?pane_dead_signal,sig#{pane_dead_signal},#{pane_dead_status}}"

// exitCode turns a deadStatus value into an exit code; a signal counts as 128+number, like a shell does.
func exitCode(s string) int {
	sig, ok := strings.CutPrefix(s, "sig")
	if !ok {
		n, _ := strconv.Atoi(s)
		return n
	}
	if n, err := strconv.Atoi(sig); err == nil {
		return 128 + n
	}
	// ponytail: the signals that end commands in practice; any other still reads as failed (128)
	nums := map[string]int{"hup": 1, "int": 2, "quit": 3, "abrt": 6, "kill": 9, "segv": 11, "pipe": 13, "term": 15}
	return 128 + nums[strings.ToLower(sig)]
}

// isShell reports whether cmd (a pane's current command) is a shell, i.e. nothing but a shell is left in the
// pane: the basename of $SHELL, sh, bash, zsh or fish.
func isShell(cmd string) bool {
	switch cmd {
	case "sh", "bash", "zsh", "fish", filepath.Base(os.Getenv("SHELL")):
		return true
	}
	return false
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
	if out, err := m.run(args...); err != nil {
		return fmt.Errorf("tmux: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (m *Manager) Kill(name string) error {
	_ = os.Remove(statePath(name))
	_, err := m.run("kill-session", "-t", "="+name)
	return err
}

// Interrupt stops the command running in the session and keeps the session (remain-on-exit), so its output
// and exit status stay: SIGTERM, or SIGKILL with force. The pane runs an interactive shell, which ignores
// SIGTERM, so the signal goes to the process group of each of its children (with job control each job leads
// its own group). Without children the pane runs the command itself and its own group is signalled.
// A dead pane is not signalled and reports signalled false; a process that is gone by the time it is
// signalled is not an error.
func (m *Manager) Interrupt(name string, force bool) (signalled bool, err error) {
	out, err := m.run("display-message", "-p", "-t", "="+name+":", "#{pane_dead} #{pane_pid}")
	if err != nil {
		return false, fmt.Errorf("tmux: %s", strings.TrimSpace(string(out)))
	}
	dead, pidStr, _ := strings.Cut(strings.TrimSpace(string(out)), " ")
	if dead == "1" {
		return false, nil
	}
	pane, err := strconv.Atoi(pidStr)
	if err != nil || pane <= 1 {
		return false, fmt.Errorf("tmux: bad pane pid %q", pidStr)
	}
	ps, err := m.ps()
	if err != nil {
		return false, fmt.Errorf("ps: %w", err)
	}
	var groups []int
	for _, line := range strings.Split(string(ps), "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || f[1] != strconv.Itoa(pane) {
			continue
		}
		if g, err := strconv.Atoi(f[2]); err == nil && g > 1 && !slices.Contains(groups, g) {
			groups = append(groups, g)
		}
	}
	if len(groups) == 0 {
		groups = []int{pane}
	}
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	var first error
	for _, g := range groups {
		if err := m.signal(g, sig); err != nil && !errors.Is(err, syscall.ESRCH) && first == nil {
			first = fmt.Errorf("signal %d: %w", g, err)
		}
	}
	return first == nil, first
}

// Prune kills the dead sessions of a fresh listing (with failedOnly, only those with a non-zero exit) and
// never touches running ones. A failing kill does not stop the others: it returns the number of sessions
// killed and the first error.
func (m *Manager) Prune(failedOnly bool) (int, error) {
	infos, err := m.List()
	if err != nil {
		return 0, err
	}
	n := 0
	var first error
	for _, s := range infos {
		if !s.Dead || (failedOnly && s.Exit == 0) {
			continue
		}
		if err := m.Kill(s.Name); err != nil {
			if first == nil {
				first = fmt.Errorf("killing %s: %w", s.Name, err)
			}
			continue
		}
		n++
	}
	return n, first
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

// Count tallies the live sessions: running (no Claude status, or working), asking and waiting.
// Dead sessions are not counted.
func Count(infos []Info) (run, ask, wait int) {
	for _, s := range infos {
		switch {
		case s.Dead:
		case s.Claude == "asking":
			ask++
		case s.Claude == "waiting":
			wait++
		default:
			run++
		}
	}
	return run, ask, wait
}

// StatusLine is the `lanes status` output: "? <asking> ● <waiting>", zero parts omitted, empty when
// nothing needs input. No colour, so it fits tmux status bars and shell prompts.
func StatusLine(infos []Info) string {
	_, ask, wait := Count(infos)
	var parts []string
	if ask > 0 {
		parts = append(parts, fmt.Sprintf("? %d", ask))
	}
	if wait > 0 {
		parts = append(parts, fmt.Sprintf("● %d", wait))
	}
	return strings.Join(parts, " ")
}
