package ui

import (
	"testing"
	"time"
)

func TestSessPollDue(t *testing.T) {
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		onSess bool
		agent  bool
		ago    time.Duration // since the last poll started; negative = never polled
		want   bool
	}{
		{"idle, polled a second ago", false, false, time.Second, false},
		{"idle, polled two seconds ago", false, false, 2 * time.Second, false},
		{"idle, interval elapsed", false, false, sessPollIdle, true},
		{"idle, interval elapsed minus tick jitter", false, false, sessPollIdle - 100*time.Millisecond, true},
		{"idle, never polled", false, false, -1, true},
		{"sessions tab, polled a second ago", true, false, time.Second, true},
		{"agent view, polled a second ago", false, true, time.Second, true},
		{"sessions tab, polled just now", true, false, 100 * time.Millisecond, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := &Model{views: []*view{{}, {sessions: true}}}
			if tc.onSess {
				m.cur = 1
			}
			if tc.agent {
				m.agents = &agentView{}
			}
			if tc.ago >= 0 {
				m.lastPoll = start.Add(-tc.ago)
			}
			if got := m.sessPollDue(start); got != tc.want {
				t.Errorf("sessPollDue = %v, want %v", got, tc.want)
			}
		})
	}
}
