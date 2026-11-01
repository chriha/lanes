package flowview

import "sort"

// Point is a cell on the canvas.
type Point struct{ X, Y int }

// elbow is the connector from a parent at from (the cell below its bottom center) to a child at to (the cell
// above its top center): down to the row above to, across, and down into to. Packets travel along it.
func elbow(from, to Point) []Point {
	return turn([]Point{from}, max(to.Y-1, from.Y), to)
}

// turn extends p from its last cell down to row mid, across to to's column and down into to.
func turn(p []Point, mid int, to Point) []Point {
	cur := p[len(p)-1]
	for y := cur.Y + 1; y <= mid; y++ {
		p = append(p, Point{cur.X, y})
	}
	step := 1
	if to.X < cur.X {
		step = -1
	}
	for x := cur.X; x != to.X; {
		x += step
		p = append(p, Point{x, max(mid, cur.Y)})
	}
	for y := max(mid, cur.Y) + 1; y <= to.Y; y++ {
		p = append(p, Point{to.X, y})
	}
	return p
}

// route is the connector from from to to that passes the boxes between them through the gaps of their rows,
// staying inside area: above each row of boxes it turns into the gap nearest to where it is, so the connectors of
// one parent share a gap, runs down through it, and ends in an elbow into to. Without boxes between it is an elbow.
func route(from, to Point, boxes []Rect, area Rect) []Point {
	rows := map[int][]Rect{} // boxes strictly between from and to, by top row
	var tops []int
	for _, b := range boxes {
		if b.Y > from.Y && b.Y+b.H <= to.Y {
			if rows[b.Y] == nil {
				tops = append(tops, b.Y)
			}
			rows[b.Y] = append(rows[b.Y], b)
		}
	}
	sort.Ints(tops)
	p := []Point{from}
	for _, y := range tops {
		row := rows[y]
		x, ok := gapNear(row, p[len(p)-1].X, area)
		if !ok {
			continue // no gap: run straight through, the box draws over the connector
		}
		bottom := y
		for _, b := range row {
			bottom = max(bottom, b.Y+b.H)
		}
		p = turn(p, max(y-2, p[len(p)-1].Y), Point{x, bottom})
	}
	return turn(p, max(to.Y-1, p[len(p)-1].Y), to)
}

// gapNear returns the free column of row nearest to x: the middle of a gap between two boxes, or one cell past
// either end of the row, inside area.
func gapNear(row []Rect, x int, area Rect) (int, bool) {
	sort.Slice(row, func(a, b int) bool { return row[a].X < row[b].X })
	cands := []int{row[0].X - 2}
	for i := 1; i < len(row); i++ {
		if end := row[i-1].X + row[i-1].W; row[i].X > end {
			cands = append(cands, end+(row[i].X-end)/2)
		}
	}
	last := row[len(row)-1]
	cands = append(cands, last.X+last.W+1)
	best, ok := 0, false
	for _, c := range cands {
		if c < area.X || c >= area.X+area.W {
			continue
		}
		if !ok || abs(c-x) < abs(best-x) {
			best, ok = c, true
		}
	}
	return best, ok
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
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
