package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/session"
)

// sessionTmux puts a tmux on PATH that keeps one session, work~1, in a state file: state is "" (none), "dead" or
// "alive". new-session fails with duplicate session while it exists, kill-session removes it (or fails with
// failKill), list-panes lists it. Each call is logged; TMUX is set so a jump takes the popup path. It returns the
// log's path.
func sessionTmux(t *testing.T, state string, failKill bool) string {
	t.Helper()
	dir := t.TempDir()
	log, st := filepath.Join(dir, "calls"), filepath.Join(dir, "state")
	if state != "" {
		if err := os.WriteFile(st, []byte(state+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	kill := "rm -f " + shq(st)
	if failKill {
		kill = "echo 'kill failed' >&2; exit 1"
	}
	script := versionAnswers("3.8", "3.8") +
		"[ \"$1\" = -L ] && shift 4\n" +
		"echo \"$*\" >> " + shq(log) + "\n" +
		"case \"$1\" in\n" +
		"new-session) if [ -f " + shq(st) + " ]; then echo \"duplicate session: $4\" >&2; exit 1; fi; echo alive > " + shq(st) + " ;;\n" +
		"kill-session) " + kill + " ;;\n" +
		"list-panes) [ -f " + shq(st) + " ] || exit 0; d=0; [ \"$(cat " + shq(st) + ")\" = dead ] && d=1; printf 'work~1\\t%s\\t0\\t1\\tclaude\\t0\\n' \"$d\" ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TMUX", "fake")
	return log
}

// A start whose listing missed the existing session of the action (lanes saw none) does not fail with duplicate
// session: a dead session is restarted, a live one is reported as running and jumped to when the action jumps.
func TestStartOverExistingSession(t *testing.T) {
	tests := []struct {
		name       string
		state      string // the work~1 session tmux has
		jump       bool
		failKill   bool
		wantNotice string   // exact, unless wantIn is set
		wantIn     []string // substrings of the notice
		wantLevel  level
		want       []string // the tmux commands, in order
	}{
		{name: "dead: restarted", state: "dead",
			wantNotice: "started work on 1 item(s)", wantLevel: lvInfo,
			want: []string{"new-session", "list-panes", "kill-session", "new-session"}},
		{name: "dead, jumps: restarted and attached", state: "dead", jump: true,
			wantNotice: "started work on 1 item(s)", wantLevel: lvInfo,
			want: []string{"new-session", "list-panes", "kill-session", "new-session", "display-popup"}},
		{name: "alive: already running", state: "alive",
			wantNotice: "work already running", wantLevel: lvWarn,
			want: []string{"new-session", "list-panes"}},
		{name: "alive, jumps: attached", state: "alive", jump: true,
			wantNotice: "work already running", wantLevel: lvWarn,
			want: []string{"new-session", "list-panes", "display-popup"}},
		{name: "dead, kill fails: the footer shows both errors", state: "dead", failKill: true,
			wantIn: []string{"duplicate session: work~1", "kill failed"}, wantLevel: lvErr,
			want: []string{"new-session", "list-panes", "kill-session", "new-session"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := busyModel(t)
			log := sessionTmux(t, tt.state, tt.failKill)
			mgr, err := session.NewManager()
			if err != nil {
				t.Fatal(err)
			}
			m.sess = mgr // m.sessByItem stays empty: the listing missed the session
			settle(m, m.runAction("work", config.Action{Run: "echo work", Jump: tt.jump}))

			if tt.wantIn == nil && m.notice != tt.wantNotice {
				t.Errorf("notice = %q, want %q", m.notice, tt.wantNotice)
			}
			for _, w := range tt.wantIn {
				if !strings.Contains(m.notice, w) {
					t.Errorf("notice = %q, want it to contain %q", m.notice, w)
				}
			}
			if m.level != tt.wantLevel {
				t.Errorf("level = %v, want %v", m.level, tt.wantLevel)
			}
			raw, _ := os.ReadFile(log)
			var got []string
			for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
				if f := strings.Fields(l); len(f) > 0 {
					got = append(got, f[0])
				}
			}
			if strings.Join(got, " ") != strings.Join(tt.want, " ") {
				t.Errorf("tmux calls = %q, want %q\n%s", got, tt.want, raw)
			}
			if _, busy := m.busy[session.Key("1")]; busy {
				t.Error("item 1 still busy")
			}
		})
	}
}
