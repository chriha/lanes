package flowview

import "testing"

func TestCenterSanitizesAndFits(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"short text is centered", "ab", "   ab   "},
		{"escape sequences are stripped", "\x1b[31mab\x1b[0m", "   ab   "},
		{"long text is cut with an ellipsis", "abcdefghij", " abcde… "},
		{"wide runes count one cell each", "日本", "   ??   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(8, 1, nil)
			c.center(Rect{0, 0, 8, 1}, 0, tt.in, RoleText)
			if got := c.String(); got != tt.want {
				t.Errorf("center = %q, want %q", got, tt.want)
			}
		})
	}
}
