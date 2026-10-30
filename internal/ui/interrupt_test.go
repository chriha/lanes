package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chriha/lanes/internal/history"
	"github.com/chriha/lanes/internal/session"
)

// paneTmux puts a tmux and a ps on PATH: tmux logs each call and reports every pane as alive with pid 4242,
// ps gives that pane one child in process group 99999999, beyond any pid limit, so the real signal Interrupt
// sends reaches no process (ESRCH, which counts as gone). exit is the fake tmux's exit status. It returns the
// tmux log's path.
func paneTmux(t *testing.T, exit int) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	fakes := map[string]string{
		"tmux": "#!/bin/sh\necho \"$*\" >> " + shq(log) + "\necho '0 4242'\nexit " + string(rune('0'+exit)) + "\n",
		"ps":   "#!/bin/sh\necho '99999999 4242 99999999'\n",
	}
	for name, script := range fakes {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func TestInterruptTargets(t *testing.T) {
	run := func(key string) session.Info {
		return session.Info{Name: "fix~" + key, Action: "fix", ItemKey: key}
	}
	dead := func(key string) session.Info {
		return session.Info{Name: "fix~" + key, Action: "fix", ItemKey: key, Dead: true, Exit: 1}
	}
	tests := []struct {
		name    string
		sess    []session.Info
		marked  []string
		prompt  string   // empty: no prompt
		notice  string   // after the key (no prompt) or after y
		signals []string // sessions Interrupt is called on
	}{
		{name: "cursor item running", sess: []session.Info{run("1")},
			prompt: "Interrupt 1 running session(s)?", notice: "interrupted 1 session(s)", signals: []string{"fix~1"}},
		{name: "marked items, dead skipped and not counted", sess: []session.Info{run("1"), dead("2"), run("3")},
			marked: []string{"1", "2", "3"},
			prompt: "Interrupt 2 running session(s)?", notice: "interrupted 2 session(s)", signals: []string{"fix~1", "fix~3"}},
		{name: "marks win over the cursor item", sess: []session.Info{run("1"), run("2")}, marked: []string{"2"},
			prompt: "Interrupt 1 running session(s)?", notice: "interrupted 1 session(s)", signals: []string{"fix~2"}},
		{name: "only dead sessions", sess: []session.Info{dead("1")}, notice: "no running sessions here"},
		{name: "no sessions", notice: "no running sessions here"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := busyModel(t)
			log := paneTmux(t, 0)
			mgr, err := session.NewManager()
			if err != nil {
				t.Fatal(err)
			}
			m.sess = mgr
			m.sessions = tt.sess
			for _, s := range tt.sess {
				m.sessByItem[s.ItemKey] = append(m.sessByItem[s.ItemKey], s)
			}
			for _, id := range tt.marked {
				m.marks[id] = true
			}
			cmd := press(m, "X")
			if tt.prompt == "" {
				if m.mode == modeConfirm || cmd != nil {
					t.Fatalf("asked %v / command %v, want neither", m.mode == modeConfirm, cmd != nil)
				}
			} else {
				if m.mode != modeConfirm || m.confirm.prompt != tt.prompt {
					t.Fatalf("mode %d, confirm %+v; want prompt %q", m.mode, m.confirm, tt.prompt)
				}
				settle(m, press(m, "y"))
			}
			if m.notice != tt.notice {
				t.Errorf("notice = %q, want %q", m.notice, tt.notice)
			}
			raw, _ := os.ReadFile(log)
			var got []string
			for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
				if _, rest, ok := strings.Cut(l, " display-message -p -t ="); ok {
					got = append(got, strings.TrimSuffix(strings.Fields(rest)[0], ":"))
				}
			}
			if strings.Join(got, ",") != strings.Join(tt.signals, ",") {
				t.Errorf("interrupted %q, want %q\ntmux calls:\n%s", got, tt.signals, raw)
			}
			for _, name := range tt.signals {
				key := strings.TrimPrefix(name, "fix~")
				evs := history.For(key)
				if len(evs) == 0 || evs[len(evs)-1].Event != "interrupted" || evs[len(evs)-1].Action != "fix" {
					t.Errorf("history of %s = %+v, want a last fix interrupted event", key, evs)
				}
			}
		})
	}
}

func TestInterruptCancelled(t *testing.T) {
	m, _ := busyModel(t)
	log := paneTmux(t, 0)
	m.sessions = []session.Info{{Name: "fix~1", Action: "fix", ItemKey: "1"}}
	m.sessByItem["1"] = m.sessions
	press(m, "X")
	if cmd := press(m, "n"); cmd != nil {
		t.Error("n returned a command")
	}
	if raw, _ := os.ReadFile(log); len(raw) != 0 {
		t.Errorf("tmux called after n:\n%s", raw)
	}
	if m.interrupted["fix~1"] {
		t.Error("cancelled session counts as interrupted")
	}
}

func TestInterruptError(t *testing.T) {
	m, _ := busyModel(t)
	paneTmux(t, 1)
	mgr, err := session.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	m.sess = mgr
	m.sessions = []session.Info{{Name: "fix~1", Action: "fix", ItemKey: "1"}}
	m.sessByItem["1"] = m.sessions
	press(m, "X")
	settle(m, press(m, "y"))
	if !strings.HasPrefix(m.notice, "✗ interrupt fix~1: ") {
		t.Errorf("notice = %q, want the error", m.notice)
	}
	if evs := history.For("1"); len(evs) != 0 {
		t.Errorf("history = %+v, want no interrupted event for a failed interrupt", evs)
	}
	if m.interrupted["fix~1"] {
		t.Error("failed interrupt remembered: the next X would send SIGKILL")
	}
}

// A second X on a session still alive after an earlier X of this run escalates to SIGKILL; once the session
// is seen dead (or gone) it is forgotten, so a restarted session of the same name starts with SIGTERM again.
func TestInterruptAgainForces(t *testing.T) {
	m, _ := busyModel(t)
	paneTmux(t, 0)
	mgr, err := session.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	m.sess = mgr
	alive := session.Info{Name: "fix~1", Action: "fix", ItemKey: "1"}
	m.sessions = []session.Info{alive}
	m.sessByItem["1"] = m.sessions
	press(m, "X")
	settle(m, press(m, "y"))
	if !m.interrupted["fix~1"] {
		t.Fatal("first X did not remember the session")
	}
	m.polling = false
	m.onSessions(sessionsMsg{infos: []session.Info{alive}})
	if !m.interrupted["fix~1"] {
		t.Fatal("still alive session forgotten: the next X would not force")
	}
	dead := alive
	dead.Dead, dead.Exit = true, 143
	m.onSessions(sessionsMsg{infos: []session.Info{dead}})
	if m.interrupted["fix~1"] {
		t.Error("dead session still remembered: a restart would be force-killed on X")
	}
}

func TestSessionsScreenInterrupt(t *testing.T) {
	m := sessModel(t, session.Info{Name: "fix~1", Action: "fix", ItemKey: "1"}, session.Info{Name: "fix~2", Action: "fix", ItemKey: "2"})
	log := paneTmux(t, 0)
	mgr, err := session.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	m.sess = mgr
	press(m, "S", "j", "X")
	if m.mode != modeConfirm || m.confirm.prompt != "Interrupt 1 running session(s)?" {
		t.Fatalf("mode %d, confirm %+v; want the interrupt prompt", m.mode, m.confirm)
	}
	cmd := press(m, "y")
	if m.mode != modeSessions {
		t.Errorf("mode = %d, want back on the sessions screen", m.mode)
	}
	runAll(cmd)
	if raw, _ := os.ReadFile(log); !strings.Contains(string(raw), "display-message -p -t =fix~2: ") {
		t.Errorf("tmux calls:\n%s\nwant the selected session fix~2 interrupted", raw)
	}
}

func TestXIsBuiltin(t *testing.T) {
	if !strings.Contains(builtinKeys, "X") {
		t.Error("X is not in builtinKeys")
	}
}
