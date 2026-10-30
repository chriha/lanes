package ui

import (
	"strings"
	"testing"
)

func TestOpenKeyOnlyOpensWebURLs(t *testing.T) {
	tests := []struct {
		name, url string
		wantOpen  bool
	}{
		{"https", "https://example.com/a?b=1", true},
		{"http", "http://example.com", true},
		{"scheme case-insensitive", "HTTPS://example.com", true},
		{"file", "file:///etc/passwd", false},
		{"javascript", "javascript:alert(1)", false},
		{"smb", "smb://host/share", false},
		{"custom scheme", "slack://open", false},
		{"scheme-less", "example.com/page", false},
		{"empty", "", false},
		{"leading dash", "-a Calculator", false},
		{"no host", "https://", false},
		{"unparsable", "http://a b\x7f", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := busyModel(t)
			m.views[m.cur].items[0].URL = tt.url
			m.cursor = 0
			var got []string
			m.open = func(u string) error { got = append(got, u); return nil }
			press(m, "o")
			if tt.wantOpen {
				if len(got) != 1 || got[0] != tt.url {
					t.Errorf("opened %q, want %q", got, tt.url)
				}
				return
			}
			if len(got) != 0 {
				t.Errorf("opened %q, want nothing", got)
			}
			if !strings.Contains(m.notice, "not opened") {
				t.Errorf("notice %q lacks reason", m.notice)
			}
		})
	}
}
