package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Default intervals (newBackoffModel sets no settings): poll_interval 30s, poll_interval_blurred 5m.

func TestTickPollsByFocus(t *testing.T) {
	tests := []struct {
		name    string
		focus   []tea.Msg     // focus messages before the tick
		age     time.Duration // time since the provider's last fetch
		backoff bool          // the provider is waiting out a failure
		want    bool          // the tick fetches
	}{
		{"no focus messages: fresh data waits", nil, 10 * time.Second, false, false},
		{"no focus messages: fetched after poll_interval", nil, 31 * time.Second, false, true},
		{"blurred: not fetched after poll_interval", []tea.Msg{tea.BlurMsg{}}, time.Minute, false, false},
		{"blurred: just under poll_interval_blurred", []tea.Msg{tea.BlurMsg{}}, 4*time.Minute + 50*time.Second, false, false},
		{"blurred: fetched after poll_interval_blurred", []tea.Msg{tea.BlurMsg{}}, 5*time.Minute + time.Second, false, true},
		{"blurred: backoff still applies", []tea.Msg{tea.BlurMsg{}}, 6 * time.Minute, true, false},
		{"focused again: back to poll_interval", []tea.Msg{tea.BlurMsg{}, tea.FocusMsg{}}, 31 * time.Second, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBackoffModel(t)
			for _, msg := range tt.focus {
				m.Update(msg)
			}
			// what the focus messages fetched is not under test here
			m.loading["p"] = false
			m.fetchedAt["p"] = time.Now().Add(-tt.age)
			if tt.backoff {
				m.retryAt["p"] = time.Now().Add(time.Minute)
			}
			m.onTick()
			if got := m.loading["p"]; got != tt.want {
				t.Errorf("tick fetched = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFocusFetchesStaleProviders(t *testing.T) {
	tests := []struct {
		name    string
		age     time.Duration
		backoff bool
		want    bool
	}{
		{"older than poll_interval: fetched at once", time.Minute, false, true},
		{"fresher than poll_interval: not fetched", 10 * time.Second, false, false},
		{"in backoff: not fetched", time.Minute, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBackoffModel(t)
			m.Update(tea.BlurMsg{})
			m.fetchedAt["p"] = time.Now().Add(-tt.age)
			if tt.backoff {
				m.retryAt["p"] = time.Now().Add(time.Minute)
			}
			_, cmd := m.Update(tea.FocusMsg{})
			if got := m.loading["p"]; got != tt.want {
				t.Errorf("focus fetched = %v, want %v", got, tt.want)
			}
			if tt.want && cmd == nil {
				t.Error("focus started a fetch but returned no command to run it")
			}
		})
	}
}

func TestFocusSkipsLoadingProvider(t *testing.T) {
	m := newBackoffModel(t)
	m.Update(tea.BlurMsg{})
	m.fetchedAt["p"] = time.Now().Add(-time.Minute)
	m.loading["p"] = true
	if _, cmd := m.Update(tea.FocusMsg{}); cmd != nil {
		t.Error("focus started a second fetch for a provider already loading")
	}
}
