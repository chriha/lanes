package ui

import (
	"os"
	"os/exec"
	"testing"
)

func TestOnTerminalGivesTheChildTheTerminalFile(t *testing.T) {
	term, err := os.CreateTemp(t.TempDir(), "tty")
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	other, err := os.CreateTemp(t.TempDir(), "other")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()

	tests := []struct {
		name    string
		syncOut *SyncOut
		stdout  *os.File // preset on the command; nil: unset
		want    *os.File // nil: stays unset
	}{
		{"unset stdout becomes the terminal", NewSyncOut(term), nil, term},
		{"a stdout the caller set is kept", NewSyncOut(term), other, other},
		{"no program output leaves it alone", nil, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBackoffModel(t)
			m.syncOut = tt.syncOut
			cmd := exec.Command("true")
			if tt.stdout != nil {
				cmd.Stdout = tt.stdout
			}
			got := m.onTerminal(cmd).Stdout
			if tt.want == nil {
				if got != nil {
					t.Fatalf("Stdout = %v, want unset", got)
				}
				return
			}
			f, ok := got.(*os.File)
			if !ok || f != tt.want {
				t.Fatalf("Stdout = %T %v, want the *os.File %v", got, got, tt.want.Name())
			}
		})
	}
}
