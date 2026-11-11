package item

import "time"

// Detail is what the detail screen fetches for one item on demand: data too costly for the list poll.
type Detail struct {
	Description string
	Pipeline    *Pipeline // nil when the item has none
	Threads     []Thread
	Comments    []Comment
}

// Pipeline is an item's latest pipeline with all its jobs.
type Pipeline struct {
	ID       string
	Status   string
	Duration time.Duration
	Jobs     []Job
}

// Job is one pipeline job; jobs are listed grouped by Stage.
type Job struct {
	Name     string
	Stage    string
	Status   string // success | failed | warning | running | pending | skipped | canceled | manual
	Duration time.Duration
}

// Thread is a review discussion; its first comment opens it, the rest are replies.
type Thread struct {
	File     string // "" for a thread not on a line of code
	Line     int
	Resolved bool
	URL      string // web URL of the first comment; "" when the provider has none
	Comments []Comment
}

// Comment is one comment of a thread or of the item itself.
type Comment struct {
	Author  string
	Body    string
	Created time.Time
}
