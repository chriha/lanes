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

func TestCanvasGauge(t *testing.T) {
	tests := []struct {
		name string
		w    int
		v    float64
		want string
	}{
		{"empty", 4, 0, "░░░░"},
		{"full", 4, 1, "████"},
		{"half on a cell boundary", 4, 0.5, "██░░"},
		{"an eighth into the edge cell", 4, 0.53125, "██▏░"},
		{"seven eighths into the edge cell", 4, 0.46875, "█▉░░"},
		{"rounds to the nearest eighth", 4, 0.03, "▏░░░"},
		{"below half an eighth is empty", 4, 0.015, "░░░░"},
		{"below 0 clamps", 4, -0.5, "░░░░"},
		{"above 1 clamps", 4, 1.7, "████"},
		{"zero width draws nothing", 0, 0.5, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanvas(6, 1, nil)
			c.Gauge(1, 0, tt.w, tt.v, RoleHaiku, RoleDim)
			want := " " + tt.want
			for len([]rune(want)) < 6 {
				want += " "
			}
			if got := c.String(); got != want {
				t.Errorf("gauge = %q, want %q", got, want)
			}
		})
	}
}

func TestCanvasGaugeRoles(t *testing.T) {
	c := NewCanvas(4, 1, nil)
	c.Gauge(0, 0, 4, 0.3, RoleHaiku, RoleDim)
	for x, want := range []Role{RoleHaiku, RoleHaiku, RoleDim, RoleDim} {
		if got := c.At(x, 0).Role; got != want {
			t.Errorf("cell %d role = %d, want %d", x, got, want)
		}
	}
}

// TestCanvasGaugeSteps proves the context gauge's resolution: at every width from 12 to 30 cells, contexts 5%
// apart are drawn differently, and a higher context never draws less.
func TestCanvasGaugeSteps(t *testing.T) {
	fill := func(s string) int {
		n := 0
		for _, r := range s {
			switch {
			case r == '█':
				n += 8
			case r >= '▉' && r <= '▏':
				n += int('▏'-r) + 1
			}
		}
		return n
	}
	for w := 12; w <= 30; w++ {
		seen := map[string]int{}
		last := -1
		for ctx := 0; ctx <= 100; ctx += 5 {
			c := NewCanvas(w, 1, nil)
			c.Gauge(0, 0, w, ctxFill(ctx), RoleHaiku, RoleDim)
			g := c.String()
			if prev, ok := seen[g]; ok {
				t.Errorf("width %d: ctx %d%% and %d%% both draw %q", w, prev, ctx, g)
			}
			seen[g] = ctx
			if f := fill(g); f < last {
				t.Errorf("width %d: ctx %d%% draws less (%q) than 5%% below", w, ctx, g)
			} else {
				last = f
			}
		}
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
