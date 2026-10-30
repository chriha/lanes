package main

import "testing"

func TestUsageError(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"no args opens the TUI", nil, false},
		{"doctor", []string{"doctor"}, false},
		{"status", []string{"status"}, false},
		{"init", []string{"init"}, false},
		{"version", []string{"version"}, false},
		{"--version", []string{"--version"}, false},
		{"hook with event", []string{"hook", "Stop"}, false},
		{"doctor extra", []string{"doctor", "x"}, true},
		{"status extra", []string{"status", "x"}, true},
		{"init extra", []string{"init", "x"}, true},
		{"version extra", []string{"version", "x"}, true},
		{"--version extra", []string{"--version", "x"}, true},
		{"hook without event", []string{"hook"}, true},
		{"hook two args", []string{"hook", "a", "b"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := usageError(tt.args); (got != "") != tt.wantErr {
				t.Errorf("usageError(%q) = %q, wantErr %v", tt.args, got, tt.wantErr)
			}
		})
	}
}
