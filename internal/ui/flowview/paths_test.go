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
