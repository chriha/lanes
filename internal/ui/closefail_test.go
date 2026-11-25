package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chriha/lanes/internal/history"
	"github.com/chriha/lanes/internal/session"
)

// x closes sessions: a kill that tmux refuses is not counted as closed, is named in the footer and leaves no
// killed entry in the history.
func TestCloseReportsFailedKills(t *testing.T) {
	tests := []struct {
		name       string
		failing    []string // sessions whose kill-session fails
		wantNotice string
		wantLevel  level
		wantKilled []string // items with a killed history entry
	}{
		{name: "all killed", wantNotice: "closed 2 session(s)", wantLevel: lvInfo, wantKilled: []string{"1", "2"}},
		{name: "one fails", failing: []string{"work~2"}, wantLevel: lvErr, wantKilled: []string{"1"},
			wantNotice: "closed 1 session(s), failed 1: tmux: refused work~2"},
		{name: "all fail", failing: []string{"work~1", "work~2"}, wantLevel: lvErr,
			wantNotice: "closed 0 session(s), failed 2: tmux: refused work~1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := sessModel(t)
			dir := t.TempDir()
			var fail string
			for _, n := range tt.failing {
				fail += "[ \"$3\" = =" + n + " ] && { echo 'refused " + n + "' >&2; exit 1; }\n"
			}
			script := versionAnswers("3.8", "3.8") +
				"[ \"$1\" = -L ] && shift 4\n" +
				"case \"$1\" in\nkill-session)\n" + fail + "exit 0 ;;\nesac\n"
			if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			mgr, err := session.NewManager()
			if err != nil {
				t.Fatal(err)
			}
			m.sess = mgr
			infos := []session.Info{
				{Name: "work~1", Action: "work", ItemKey: "1"},
				{Name: "work~2", Action: "work", ItemKey: "2"},
			}
			m.closeAndDrop(infos, nil)
			if m.confirm == nil {
				t.Fatal("no confirmation asked")
			}
			msg, ok := m.confirm.do()().(doneMsg)
			if !ok {
				t.Fatal("kill command did not return a doneMsg")
			}
			if msg.notice != tt.wantNotice || msg.level != tt.wantLevel {
				t.Errorf("notice = %q (level %d), want %q (level %d)", msg.notice, msg.level, tt.wantNotice, tt.wantLevel)
			}
			var killed []string
			for _, k := range []string{"1", "2"} {
				for _, e := range history.For(k) {
					if e.Event == "killed" {
						killed = append(killed, k)
					}
				}
			}
			if strings.Join(killed, ",") != strings.Join(tt.wantKilled, ",") {
				t.Errorf("killed history for items %v, want %v", killed, tt.wantKilled)
			}
		})
	}
}
