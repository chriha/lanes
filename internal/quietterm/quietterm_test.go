package quietterm

import (
	"os"
	"testing"
)

func TestMuteAndRestore(t *testing.T) {
	tests := []struct {
		name string
		term *string // nil = TERM unset
	}{
		{"set", ptr("xterm-256color")},
		{"empty", ptr("")},
		{"unset", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TERM", "placeholder") // registers the restore of the real value
			if tt.term == nil {
				if err := os.Unsetenv("TERM"); err != nil {
					t.Fatal(err)
				}
			} else {
				t.Setenv("TERM", *tt.term)
			}
			mute()
			if got := os.Getenv("TERM"); got != "dumb" {
				t.Errorf("TERM while muted = %q, want dumb", got)
			}
			Restore()
			got, ok := os.LookupEnv("TERM")
			if tt.term == nil && ok {
				t.Errorf("TERM restored to %q, want unset", got)
			}
			if tt.term != nil && (!ok || got != *tt.term) {
				t.Errorf("TERM restored to %q (set %v), want %q", got, ok, *tt.term)
			}
		})
	}
}

func ptr(s string) *string { return &s }
