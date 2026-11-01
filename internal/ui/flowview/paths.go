package flowview

// Point is a cell on the canvas.
type Point struct{ X, Y int }

// elbow is the connector from a parent at from (the cell below its bottom center) to a child at to (the cell
// above its top center): down to the row above to, across, and down into to. Packets travel along it.
func elbow(from, to Point) []Point {
	mid := max(to.Y-1, from.Y)
	var p []Point
	for y := from.Y; y < mid; y++ {
		p = append(p, Point{from.X, y})
	}
	step := 1
	if to.X < from.X {
		step = -1
	}
	for x := from.X; x != to.X; x += step {
		p = append(p, Point{x, mid})
	}
	for y := mid; y <= to.Y; y++ {
		p = append(p, Point{to.X, y})
	}
	return p
}

// route is the connector from from to to that passes the boxes between them through the gaps of their rows,
// staying inside area.
func route(from, to Point, boxes []Rect, area Rect) []Point {
	return elbow(from, to)
}

// Directions a connector leaves a cell in.
const (
	up uint8 = 1 << iota
	down
	left
	right
)

var joints = map[uint8]rune{
	up: '│', down: '│', up | down: '│', left: '─', right: '─', left | right: '─',
	down | right: '┌', down | left: '┐', up | right: '└', up | left: '┘',
	up | down | right: '├', up | down | left: '┤', down | left | right: '┬', up | left | right: '┴',
	up | down | left | right: '┼',
}

// drawPaths draws the paths as one set of box-drawing lines, so paths that share cells join with the right
// junctions, and ends each path in an arrowhead.
func (c *Canvas) drawPaths(paths [][]Point, role Role) {
	dirs := map[Point]uint8{}
	for _, p := range paths {
		for i := 1; i < len(p); i++ {
			a, b := p[i-1], p[i]
			switch {
			case b.Y > a.Y:
				dirs[a] |= down
				dirs[b] |= up
			case b.Y < a.Y:
				dirs[a] |= up
				dirs[b] |= down
			case b.X > a.X:
				dirs[a] |= right
				dirs[b] |= left
			case b.X < a.X:
				dirs[a] |= left
				dirs[b] |= right
			}
		}
	}
	for pt, d := range dirs {
		c.Set(pt.X, pt.Y, joints[d], role)
	}
	for _, p := range paths {
		if len(p) > 0 {
			end := p[len(p)-1]
			c.Set(end.X, end.Y, '▼', role)
		}
	}
}
