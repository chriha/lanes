package flowview

import (
	"strings"
	"testing"
)

func TestCanvasSegments(t *testing.T) {
	tests := []struct {
		name  string
		v     float64
		blink bool
		want  string
	}{
		{"zero shows none", 0, false, "□□□□□□□□□□"},
		{"4% rounds up to one", 0.04, false, "■□□□□□□□□□"},
		{"1% shows one", 0.01, false, "■□□□□□□□□□"},
		{"14% rounds down to one", 0.14, false, "■□□□□□□□□□"},
		{"15% rounds up to two", 0.15, false, "■■□□□□□□□□"},
		{"50%", 0.5, false, "■■■■■□□□□□"},
		{"100%", 1, false, "■■■■■■■■■■"},
		{"below 0 clamps", -0.5, false, "□□□□□□□□□□"},
		{"above 1 clamps", 1.7, false, "■■■■■■■■■■"},
		{"blink hides the last filled", 0.5, true, "■■■■□□□□□□"},
		{"blink at 100%", 1, true, "■■■■■■■■■□"},
		{"blink with none filled changes nothing", 0, true, "□□□□□□□□□□"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(12, 1, nil)
			c.Segments(1, 0, '■', tt.v, tt.blink, RoleHaiku, RoleDim)
			// filled cells are ■ in fill, the rest ■ in track; compare by role
			var got strings.Builder
			got.WriteRune(' ')
			for x := 1; x <= segmentCount; x++ {
				cell := c.At(x, 0)
				if cell.R != '■' {
					t.Fatalf("cell %d = %q, want ■", x, cell.R)
				}
				if cell.Role == RoleHaiku {
					got.WriteRune('■')
				} else if cell.Role == RoleDim {
					got.WriteRune('□')
				} else {
					t.Fatalf("cell %d role %d", x, cell.Role)
				}
			}
			if want := " " + tt.want; got.String() != want {
				t.Errorf("segments = %q, want %q", got.String(), want)
			}
			if c.At(0, 0).R != ' ' && c.At(0, 0).R != 0 || c.At(11, 0).R == '■' {
				t.Errorf("drew outside the 10 cells: %q", c.String())
			}
		})
	}
}

func TestCanvasSegmentsGlyph(t *testing.T) {
	c := NewCanvas(10, 1, nil)
	c.Segments(0, 0, '▬', 0.3, false, RoleHaiku, RoleDim)
	if got := c.String(); got != "▬▬▬▬▬▬▬▬▬▬" {
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
