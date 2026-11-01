package flowview

import (
	"fmt"
	"strings"
	"testing"
)

func TestElbow(t *testing.T) {
	tests := []struct {
		name     string
		from, to Point
		want     string
	}{
		{"straight down", Point{2, 0}, Point{2, 2}, "[{2 0} {2 1} {2 2}]"},
		{"right", Point{0, 0}, Point{2, 2}, "[{0 0} {0 1} {1 1} {2 1} {2 2}]"},
		{"left", Point{2, 0}, Point{0, 2}, "[{2 0} {2 1} {1 1} {0 1} {0 2}]"},
		{"one row: across, then down", Point{0, 0}, Point{2, 1}, "[{0 0} {1 0} {2 0} {2 1}]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fmt.Sprint(elbow(tt.from, tt.to)); got != tt.want {
				t.Errorf("elbow = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestRoute(t *testing.T) {
	line := []Rect{{0, 2, 9, 5}, {12, 2, 9, 5}} // one line of boxes at rows 2-6, a gap at columns 9-11
	tests := []struct {
		name     string
		from, to Point
		boxes    []Rect
		area     Rect
		via      Point // a cell the connector passes
	}{
		{"no boxes between: an elbow", Point{4, 0}, Point{16, 1}, line, Rect{0, 0, 40, 12}, Point{16, 0}},
		{"through the gap nearest the parent", Point{4, 0}, Point{30, 10}, line, Rect{0, 0, 40, 12}, Point{10, 4}},
		{"past the row's end when nearer", Point{30, 0}, Point{4, 10}, line, Rect{0, 0, 40, 12}, Point{22, 4}},
		{"never outside the area", Point{4, 0}, Point{4, 10}, line[:1], Rect{0, 0, 20, 12}, Point{10, 4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := route(tt.from, tt.to, tt.boxes, tt.area)
			if p[0] != tt.from || p[len(p)-1] != tt.to {
				t.Fatalf("route runs %v to %v, want %v to %v", p[0], p[len(p)-1], tt.from, tt.to)
			}
			found := false
			for k, pt := range p {
				if pt == tt.via {
					found = true
				}
				if k > 0 {
					if dx, dy := pt.X-p[k-1].X, pt.Y-p[k-1].Y; dx*dx+dy*dy != 1 {
						t.Fatalf("route jumps from %v to %v", p[k-1], pt)
					}
				}
				for _, r := range tt.boxes {
					if pt.X >= r.X && pt.X < r.X+r.W && pt.Y >= r.Y && pt.Y < r.Y+r.H {
						t.Fatalf("route crosses box %+v at %v: %v", r, pt, p)
					}
				}
			}
			if !found {
				t.Errorf("route %v does not pass %v", p, tt.via)
			}
		})
	}
}

func TestDrawPathsJoinsAFanOut(t *testing.T) {
	c := NewCanvas(9, 3, nil)
	from := Point{4, 0}
	c.drawPaths([][]Point{elbow(from, Point{0, 2}), elbow(from, Point{4, 2}), elbow(from, Point{8, 2})}, RoleDim)
	want := strings.Join([]string{
		"    │    ",
		"┌───┼───┐",
		"▼   ▼   ▼",
	}, "\n")
	if got := c.String(); got != want {
		t.Errorf("fan-out =\n%s\nwant\n%s", got, want)
	}
}
