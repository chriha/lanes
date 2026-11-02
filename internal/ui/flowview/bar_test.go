package flowview

import (
	"strings"
	"testing"
)

func TestCanvasSegments(t *testing.T) {
	tests := []struct {
		name  string
		width int
		v     float64
		want  string // ■ filled, □ track
	}{
		{"zero shows none", 10, 0, "□□□□□□□□□□"},
		{"1% shows one cell", 10, 0.01, "■□□□□□□□□□"},
		{"4% rounds to none but shows one", 10, 0.04, "■□□□□□□□□□"},
		{"14% rounds to one", 10, 0.14, "■□□□□□□□□□"},
		{"15% rounds to two", 10, 0.15, "■■□□□□□□□□"},
		{"50%", 10, 0.5, "■■■■■□□□□□"},
		{"97% rounds to full", 10, 0.97, "■■■■■■■■■■"},
		{"100%", 10, 1, "■■■■■■■■■■"},
		{"below 0 clamps", 10, -0.5, "□□□□□□□□□□"},
		{"above 1 clamps", 10, 1.7, "■■■■■■■■■■"},
		{"zero, other width", 4, 0, "□□□□"},
		{"1% shows one cell, other width", 4, 0.01, "■□□□"},
		{"50%, other width", 4, 0.5, "■■□□"},
		{"100%, other width", 4, 1, "■■■■"},
		{"wide bar, 50%", 30, 0.5, strings.Repeat("■", 15) + strings.Repeat("□", 15)},
		{"zero width draws nothing", 0, 0.5, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(tt.width+2, 1, nil)
			c.Segments(1, 0, tt.width, '■', tt.v, RoleHaiku, RoleDim)
			var got strings.Builder
			for x := 1; x <= tt.width; x++ {
				cell := c.At(x, 0)
				switch {
				case cell.R == '■' && cell.Role == RoleHaiku:
					got.WriteRune('■')
				case cell.R == '■' && cell.Role == RoleDim:
					got.WriteRune('□')
				default:
					t.Fatalf("cell %d = %q role %d", x, cell.R, cell.Role)
				}
			}
			if got.String() != tt.want {
				t.Errorf("segments = %q, want %q", got.String(), tt.want)
			}
			if c.At(0, 0).R == '■' || c.At(tt.width+1, 0).R == '■' {
				t.Errorf("drew outside the %d cells: %q", tt.width, c.String())
			}
		})
	}
}

func TestEffortMeter(t *testing.T) {
	tests := map[string]string{
		"low":     "▮▯▯▯",
		"medium":  "▮▮▯▯",
		"high":    "▮▮▮▯",
		"xhigh":   "▮▮▮▮",
		"max":     "▮▮▮▮",
		"High":    "▮▮▮▯",
		"":        "",
		"extreme": "",
	}
	for level, want := range tests {
		if got := effortMeter(level); got != want {
			t.Errorf("effortMeter(%q) = %q, want %q", level, got, want)
		}
	}
}

func TestCanvasSpark(t *testing.T) {
	tests := []struct {
		name   string
		counts []int
		want   string
	}{
		{"no counts draw nothing", nil, ""},
		{"all zero is the floor", []int{0, 0, 0}, "▁▁▁"},
		{"scaled to the highest", []int{0, 1, 2, 4, 8}, "▁▂▃▅█"},
		{"a single call is full height", []int{0, 1}, "▁█"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(len(tt.counts), 1, nil)
			c.Spark(0, 0, tt.counts, RoleHaiku, RoleDim)
			if got := c.String(); got != tt.want {
				t.Errorf("spark = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCanvasSparkRoles(t *testing.T) {
	c := NewCanvas(3, 1, nil)
	c.Spark(0, 0, []int{0, 3, 1}, RoleHaiku, RoleDim)
	for x, want := range []Role{RoleDim, RoleHaiku, RoleHaiku} {
		if got := c.At(x, 0).Role; got != want {
			t.Errorf("cell %d role = %d, want %d", x, got, want)
		}
	}
}
