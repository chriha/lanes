package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/session"
)

// fakeTmux puts a tmux on PATH that logs each call as one line and succeeds; it returns the log's path.
// TMUX is set so jumps take the popup path, which runs (the fake) tmux instead of suspending the TUI.
func fakeTmux(t *testing.T) string {
	t.Helper()
	return fakeTmuxVersions(t, "3.8", "3.8")
}

// versionAnswers is the start of a fake tmux script: it answers the client (tmux -V) and server
// (display-message -p #{version}) version queries without logging them. A server of "mismatch" fails the
// server query with tmux's protocol error.
func versionAnswers(client, server string) string {
	srv := "echo " + shq(server) + "; exit 0"
	if server == "mismatch" {
		srv = "echo 'protocol version mismatch (client 9, server 8)' >&2; exit 1"
	}
	return "#!/bin/sh\ncase \"$*\" in\n" +
		"*' -V') echo " + shq("tmux "+client) + "; exit 0;;\n" +
		"*'display-message -p #{version}') " + srv + ";;\n" +
		"esac\n"
}

// fakeTmuxVersions is fakeTmux with the given installed (client) and running server tmux versions.
func fakeTmuxVersions(t *testing.T, client, server string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	script := versionAnswers(client, server) + "echo \"$*\" >> " + shq(log) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TMUX", "fake")
	return log
}

// settle runs cmd and feeds every message it yields back into the model, until no command is left.
func settle(m *Model, cmd tea.Cmd) {
	for _, msg := range runAll(cmd) {
		if _, ok := msg.(pollNow); ok {
			continue // would poll tmux; the tests set the sessions themselves
		}
		_, next := m.Update(msg)
		settle(m, next)
	}
}

func TestJumpAction(t *testing.T) {
	const attach = "attach -t '=review~1'"
	tests := []struct {
		name     string
		jump     bool
		sess     *session.Info // existing session of item 1
		marked   []string
		want     []string // substrings of the tmux calls, in order
		noAttach bool
		notice   string // expected notice, empty = not checked
	}{
		{name: "starts then attaches", jump: true,
			want: []string{"new-session -d -s review~1", attach}},
		{name: "running: attaches, no start", jump: true,
			sess: &session.Info{Name: "review~1", Action: "review", ItemKey: "1"},
			want: []string{attach}},
		{name: "dead: restarts then attaches", jump: true,
			sess: &session.Info{Name: "review~1", Action: "review", ItemKey: "1", Dead: true},
			want: []string{"kill-session -t =review~1", "new-session -d -s review~1", attach}},
		{name: "several marked: starts all, no attach", jump: true, marked: []string{"1", "2"},
			want: []string{"new-session -d -s review~1", "new-session -d -s review~2"}, noAttach: true},
		{name: "without jump: starts, no attach",
			want: []string{"new-session -d -s review~1"}, noAttach: true},
		{name: "without jump, running: notice, no attach",
			sess: &session.Info{Name: "review~1", Action: "review", ItemKey: "1"},
			want: nil, noAttach: true, notice: "review already running"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := busyModel(t)
			log := fakeTmux(t)
			mgr, err := session.NewManager()
			if err != nil {
				t.Fatal(err)
			}
			m.sess = mgr
			if tt.sess != nil {
				m.sessions = []session.Info{*tt.sess}
				m.sessByItem["1"] = m.sessions
			}
			for _, id := range tt.marked {
				m.marks[id] = true
			}
			cmd := m.runAction("review", config.Action{Run: "echo review", Jump: tt.jump})
			if len(tt.marked) > 1 {
				if m.mode != modeConfirm {
					t.Fatal("several items did not ask first")
				}
				cmd = press(m, "y")
			}
			settle(m, cmd)

			raw, _ := os.ReadFile(log)
			calls := strings.Split(strings.TrimSpace(string(raw)), "\n")
			if len(raw) == 0 {
				calls = nil
			}
			if len(calls) != len(tt.want) {
				t.Fatalf("tmux calls:\n%s\nwant %d matching %q", raw, len(tt.want), tt.want)
			}
			for i, w := range tt.want {
				if !strings.Contains(calls[i], w) {
					t.Errorf("call %d = %q, want it to contain %q", i, calls[i], w)
				}
			}
			if tt.noAttach && strings.Contains(string(raw), " attach -t ") {
				t.Errorf("attached, want no attach:\n%s", raw)
			}
			if strings.Contains(m.notice, "already running") && tt.notice == "" {
				t.Errorf("notice = %q, want no already-running notice", m.notice)
			}
			if tt.notice != "" && m.notice != tt.notice {
				t.Errorf("notice = %q, want %q", m.notice, tt.notice)
			}
		})
	}
}

func TestRunningItemsKeepTheirMark(t *testing.T) {
	m, _ := busyModel(t)
	fakeTmux(t)
	mgr, err := session.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	m.sess = mgr
	m.sessions = []session.Info{{Name: "review~1", Action: "review", ItemKey: "1"}}
	m.sessByItem["1"] = m.sessions
	m.marks["1"], m.marks["2"] = true, true // 1 already runs, 2 starts
	m.runAction("review", config.Action{Run: "echo review"})
	if m.mode != modeConfirm {
		t.Fatal("several items did not ask first")
	}
	settle(m, press(m, "y"))
	if len(m.marks) != 1 || !m.marks["1"] {
		t.Errorf("marks = %v, want only the running item 1", m.marks)
	}
}

func TestJumpMarks(t *testing.T) {
	run := &session.Info{Name: "review~1", Action: "review", ItemKey: "1"}
	tests := []struct {
		name string
		do   func(m *Model) tea.Cmd
		want map[string]bool
	}{
		{name: "action jump into a running item clears its mark",
			do: func(m *Model) tea.Cmd {
				m.marks["1"] = true
				return m.runAction("review", config.Action{Run: "echo review", Jump: true})
			},
			want: map[string]bool{}},
		{name: "plain jump keeps the marks",
			do: func(m *Model) tea.Cmd {
				m.marks["1"], m.marks["2"] = true, true
				return m.jump(m.items()[m.cursor])
			},
			want: map[string]bool{"1": true, "2": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := busyModel(t)
			fakeTmux(t)
			mgr, err := session.NewManager()
			if err != nil {
				t.Fatal(err)
			}
			m.sess = mgr
			m.sessions = []session.Info{*run}
			m.sessByItem["1"] = m.sessions
			settle(m, tt.do(m))
			if len(m.marks) != len(tt.want) {
				t.Fatalf("marks = %v, want %v", m.marks, tt.want)
			}
			for id := range tt.want {
				if !m.marks[id] {
					t.Errorf("marks = %v, want %v", m.marks, tt.want)
				}
			}
		})
	}
}

func TestJumpAttachErrors(t *testing.T) {
	tests := []struct {
		name   string
		exit   int
		popup  bool
		notice string // substring, empty = no notice
	}{
		{name: "popup attach fails", exit: 1, popup: true, notice: "attach: "},
		{name: "popup detach is not an error", exit: 0, popup: true},
		{name: "plain attach fails", exit: 1, notice: "attach: "},
		{name: "plain detach is not an error", exit: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := busyModel(t)
			dir := t.TempDir()
			script := versionAnswers("3.8", "3.8") + fmt.Sprintf("exit %d\n", tt.exit)
			if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("TMUX", "")
			if tt.popup {
				t.Setenv("TMUX", "fake")
			}
			mgr, err := session.NewManager()
			if err != nil {
				t.Fatal(err)
			}
			m.sess = mgr
			cmd, popup := m.sess.JumpCmd("review~1")
			if popup != tt.popup {
				t.Fatalf("popup = %v, want %v", popup, tt.popup)
			}
			// the popup path runs the command itself; a plain attach is run by tea.ExecProcess, whose
			// callback is attachDone, so feed it what running the command returned
			var msg tea.Msg
			if popup {
				msg = m.jumpTo("review~1")()
				for _, ok := msg.(pollNow); !ok; _, ok = msg.(pollNow) { // the version check before the attach
					_, next := m.Update(msg)
					msg = next()
				}
			} else {
				msg = attachDone(cmd.Run())
			}
			m.Update(msg)
			if tt.notice == "" && m.notice != "" {
				t.Errorf("notice = %q, want none", m.notice)
			}
			if tt.notice != "" && (!strings.Contains(m.notice, tt.notice) || m.level != lvErr) {
				t.Errorf("notice = %q (level %v), want an error containing %q", m.notice, m.level, tt.notice)
			}
		})
	}
}

func TestJumpVersionMismatch(t *testing.T) {
	const attach = "attach -t '=review~1'"
	tests := []struct {
		name    string
		server  string // version of the running lanes server; client is 3.8
		keys    []string
		prompt  []string // substrings of the confirm prompt, nil = no prompt
		want    string   // tmux call made, empty = none
		notWant string   // tmux call that must not be made
	}{
		{name: "no mismatch attaches as before", server: "3.8", want: attach, notWant: "kill-server"},
		{name: "mismatch accepted restarts the server", server: "3.5a", keys: []string{"y"},
			prompt: []string{"3.5a", "3.8", "review~1", "shell~2"}, want: "kill-server", notWant: attach},
		{name: "mismatch declined changes nothing", server: "3.5a", keys: []string{"n"},
			prompt: []string{"3.5a", "3.8", "review~1", "shell~2"}, notWant: "kill-server"},
		{name: "protocol mismatch accepted restarts the server", server: "mismatch", keys: []string{"y"},
			prompt: []string{"3.8", "review~1"}, want: "kill-server", notWant: attach},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := busyModel(t)
			log := fakeTmuxVersions(t, "3.8", tt.server)
			mgr, err := session.NewManager()
			if err != nil {
				t.Fatal(err)
			}
			m.sess = mgr
			m.sessions = []session.Info{
				{Name: "review~1", Action: "review", ItemKey: "1"},
				{Name: "shell~2", Action: "shell", ItemKey: "2"},
			}
			m.sessByItem["1"] = m.sessions[:1]
			m.sessByItem["2"] = m.sessions[1:]
			settle(m, m.jump(m.items()[m.cursor]))

			if tt.prompt == nil && m.mode == modeConfirm {
				t.Fatalf("asked %q, want no prompt", m.confirm.prompt)
			}
			if tt.prompt != nil {
				if m.mode != modeConfirm {
					t.Fatal("no confirm before restarting the tmux server")
				}
				for _, p := range tt.prompt {
					if !strings.Contains(m.confirm.prompt, p) {
						t.Errorf("prompt %q does not mention %q", m.confirm.prompt, p)
					}
				}
				settle(m, press(m, tt.keys...))
			}
			raw, _ := os.ReadFile(log)
			if tt.want != "" && !strings.Contains(string(raw), tt.want) {
				t.Errorf("tmux calls:\n%s\nwant one containing %q", raw, tt.want)
			}
			if strings.Contains(string(raw), tt.notWant) {
				t.Errorf("tmux calls:\n%s\nwant none containing %q", raw, tt.notWant)
			}
		})
	}
}
