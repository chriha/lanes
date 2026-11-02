package flowview

import (
	"strings"
	"testing"
)

func TestCanvasSegments(t *testing.T) {
	tests := []struct {
		name string
		v    float64
		want string // ■ full, ▌ half, □ track
	}{
		{"zero shows none", 0, "□□□□□□□□□□"},
		{"2% rounds up to a half", 0.02, "▌□□□□□□□□□"},
		{"3% rounds to a half", 0.03, "▌□□□□□□□□□"},
		{"4% rounds to a half", 0.04, "▌□□□□□□□□□"},
		{"5% is a half", 0.05, "▌□□□□□□□□□"},
		{"1% shows a half", 0.01, "▌□□□□□□□□□"},
		{"10% is one full", 0.1, "■□□□□□□□□□"},
		{"14% rounds to 15%", 0.14, "■▌□□□□□□□□"},
		{"50%", 0.5, "■■■■■□□□□□"},
		{"97% rounds to 95%", 0.97, "■■■■■■■■■▌"},
		{"100%", 1, "■■■■■■■■■■"},
		{"below 0 clamps", -0.5, "□□□□□□□□□□"},
		{"above 1 clamps", 1.7, "■■■■■■■■■■"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(12, 1, nil)
			c.Segments(1, 0, '■', tt.v, RoleHaiku, RoleDim)
			var got strings.Builder
			for x := 1; x <= segmentCount; x++ {
				cell := c.At(x, 0)
				switch {
				case cell.R == '■' && cell.Role == RoleHaiku:
					got.WriteRune('■')
				case cell.R == '▌' && cell.Role == RoleHaiku:
					got.WriteRune('▌')
				case cell.R == '■' && cell.Role == RoleDim:
					got.WriteRune('□')
				default:
					t.Fatalf("cell %d = %q role %d", x, cell.R, cell.Role)
				}
			}
			if got.String() != tt.want {
				t.Errorf("segments = %q, want %q", got.String(), tt.want)
			}
			if c.At(0, 0).R != ' ' && c.At(0, 0).R != 0 || c.At(11, 0).R == '■' {
				t.Errorf("drew outside the 10 cells: %q", c.String())
			}
		})
	}
}

func TestCanvasSegmentsGlyph(t *testing.T) {
	c := NewCanvas(10, 1, nil)
	c.Segments(0, 0, '▬', 0.35, RoleHaiku, RoleDim)
	if got := c.String(); got != "▬▬▬▌▬▬▬▬▬▬" {
		t.Errorf("segments = %q", got)
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
