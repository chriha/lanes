package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSessPollDue(t *testing.T) {
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		onSess bool
		agent  bool
		blur   bool
		ago    time.Duration // since the last poll started; negative = never polled
		want   bool
	}{
		{"idle, polled a second ago", false, false, false, time.Second, false},
		{"idle, polled two seconds ago", false, false, false, 2 * time.Second, false},
		{"idle, interval elapsed", false, false, false, sessPollIdle, true},
		{"idle, interval elapsed minus tick jitter", false, false, false, sessPollIdle - 100*time.Millisecond, true},
		{"idle, never polled", false, false, false, -1, true},
		{"sessions tab, polled a second ago", true, false, false, time.Second, true},
		{"agent view, polled a second ago", false, true, false, time.Second, true},
		{"sessions tab, polled just now", true, false, false, 100 * time.Millisecond, false},
		{"blurred, polled 3s ago", false, false, true, 3 * time.Second, false},
		{"blurred, polled 5s ago", true, false, true, 5 * time.Second, false},
		{"blurred, blurred interval elapsed", true, true, true, blurredPoll, true},
		{"blurred, never polled", false, false, true, -1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := &Model{views: []*view{{}, {sessions: true}}}
			if tc.onSess {
				m.cur = 1
			}
			m.blurred = tc.blur
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

func TestTickPollsByFocusState(t *testing.T) {
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		blurred  bool
		slowAgo  time.Duration // since the usage reads and the config stat last ran
		wantSlow bool
		wantPeek bool
	}{
		{"focused: every tick", false, time.Second, true, true},
		{"blurred, ran a second ago: skipped", true, time.Second, false, false},
		{"blurred, ran 5s ago: skipped", true, 5 * time.Second, false, false},
		{"blurred, interval elapsed: due", true, blurredPoll, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := &Model{blurred: tc.blurred, lastSlow: start.Add(-tc.slowAgo)}
			if got := m.slowPollDue(start); got != tc.wantSlow {
				t.Errorf("slowPollDue = %v, want %v", got, tc.wantSlow)
			}
			if got := m.peekDue(); got != tc.wantPeek {
				t.Errorf("peekDue = %v, want %v", got, tc.wantPeek)
			}
		})
	}
}

func TestFocusRestartsSlowPolls(t *testing.T) {
	m := newBackoffModel(t)
	m.Update(tea.BlurMsg{})
	before := time.Now()
	m.Update(tea.FocusMsg{})
	if m.lastSlow.Before(before) {
		t.Errorf("focus did not restart the slow polls: lastSlow = %v", m.lastSlow)
	}
	if !m.slowPollDue(time.Now()) {
		t.Error("focused: slow polls are due every tick")
	}
}
