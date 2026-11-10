package ui

// dwellMsg fires once the cursor rested on an unread item for dwell; gen drops the ticks of a cursor that moved on.
type dwellMsg struct {
	id  string
	gen int
}
