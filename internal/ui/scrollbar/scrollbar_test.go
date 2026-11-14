package scrollbar

import "testing"

func TestRows(t *testing.T) {
	tests := []struct {
		name                           string
		offset, visible, total, length int
		want                           string
	}{
		{"fits", 0, 10, 10, 10, ""},
		{"less than visible", 0, 10, 4, 10, ""},
		{"no room", 0, 10, 20, 0, ""},
		{"top", 0, 10, 20, 10, "█████░░░░░"},
		{"bottom", 10, 10, 20, 10, "░░░░░█████"},
		{"middle", 5, 10, 20, 10, "░░░█████░░"},
		{"min one cell", 0, 1, 1000, 10, "█░░░░░░░░░"},
		{"min one cell at the end", 999, 1, 1000, 10, "░░░░░░░░░█"},
		{"past the end clamps", 99, 10, 20, 10, "░░░░░█████"},
		{"negative clamps", -5, 10, 20, 10, "█████░░░░░"},
		{"one row", 3, 5, 10, 1, "█"},
		{"proportional", 0, 25, 100, 8, "██░░░░░░"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(Rows(tt.offset, tt.visible, tt.total, tt.length))
			if got != tt.want {
				t.Errorf("Rows(%d, %d, %d, %d) = %q, want %q", tt.offset, tt.visible, tt.total, tt.length, got, tt.want)
			}
		})
	}
}
