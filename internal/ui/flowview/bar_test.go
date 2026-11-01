package flowview

import "testing"

func TestCanvasBar(t *testing.T) {
	tests := []struct {
		name  string
		w     int
		v     float64
		phase int
		want  string
	}{
		{"empty", 4, 0, 0, "░░░░"},
		{"full", 4, 1, 0, "████"},
		{"half on a cell boundary", 4, 0.5, 0, "██░░"},
		{"edge a quarter filled", 4, 0.3125, 0, "█░░░"},
		{"edge half filled", 4, 0.375, 0, "█▒░░"},
		{"edge three quarters filled", 4, 0.4375, 0, "█▓░░"},
		{"shimmer shifts the edge", 4, 0.375, 1, "█▓░░"},
		{"shimmer wraps around", 4, 0.375, 2, "█░░░"},
		{"negative phase", 4, 0.375, -1, "█░░░"},
		{"below 0 clamps", 4, -0.5, 0, "░░░░"},
		{"above 1 clamps", 4, 1.7, 2, "████"},
		{"zero width draws nothing", 0, 0.5, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(6, 1, nil)
			c.Bar(1, 0, tt.w, tt.v, tt.phase, RoleHaiku, RoleDim)
			want := " " + tt.want
			for len([]rune(want)) < 6 {
				want += " "
			}
			if got := c.String(); got != want {
				t.Errorf("bar = %q, want %q", got, want)
			}
		})
	}
}

func TestCanvasBarRoles(t *testing.T) {
	c := NewCanvas(4, 1, nil)
	c.Bar(0, 0, 4, 0.375, 0, RoleHaiku, RoleDim)
	want := []Role{RoleHaiku, RoleHaiku, RoleDim, RoleDim}
	for x, r := range want {
		if got := c.At(x, 0).Role; got != r {
			t.Errorf("cell %d role = %d, want %d", x, got, r)
		}
	}
}

func TestCanvasBarClips(t *testing.T) {
	c := NewCanvas(3, 1, nil)
	c.Bar(1, 0, 5, 1, 0, RoleHaiku, RoleDim)
	if got := c.String(); got != " ██" {
		t.Errorf("bar = %q, want %q", got, " ██")
	}
}
