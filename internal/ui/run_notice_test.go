package ui

import (
	"strings"
	"testing"
)

func TestExecuteRunNotice(t *testing.T) {
	tests := []struct {
		name        string
		cmd         string
		busy        int
		skippedWhen string
		prefix      string
		contains    []string
		ok          bool
	}{
		{"success plain", "true", 0, "", "✓ act done on 1 item(s)", nil, true},
		{"success notes", "true", 2, "skipped 3 item(s) (when)", "✓ act done on 1 item(s)", []string{"2 busy", "skipped 3 item(s) (when)"}, true},
		{"failure plain", "echo boom; exit 1", 0, "", "✗ act failed on 1/1 — ", []string{"boom"}, false},
		{"failure notes", "echo boom; exit 1", 2, "skipped 3 item(s) (when)", "✗ act failed on 1/1 — ", []string{"boom", "2 busy", "skipped 3 item(s) (when)"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			t.Setenv("SHELL", "/bin/sh")
			m := &Model{}
			jobs := []job{{label: "a", itemKey: "k", cwd: t.TempDir(), cmd: tt.cmd}}
			d, ok := m.executeRun("act", jobs, false, tt.busy, tt.skippedWhen)().(doneMsg)
			if !ok {
				t.Fatal("want doneMsg")
			}
			if !strings.HasPrefix(d.notice, tt.prefix) {
				t.Errorf("notice = %q, want prefix %q", d.notice, tt.prefix)
			}
			for _, s := range tt.contains {
				if !strings.Contains(d.notice, s) {
					t.Errorf("notice = %q, missing %q", d.notice, s)
				}
			}
			if d.ok != tt.ok {
				t.Errorf("ok = %v, want %v", d.ok, tt.ok)
			}
		})
	}
}
