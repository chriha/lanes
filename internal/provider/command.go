package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/chriha/lanes/internal/item"
)

// Command runs a shell command that prints a JSON array of items, and DetailCmd, when set, for an item's detail.
type Command struct{ Cmd, DetailCmd string }

func (c *Command) Fetch(ctx context.Context) ([]item.Item, error) {
	out, err := runShell(ctx, c.Cmd)
	if err != nil {
		return nil, err
	}
	var items []item.Item
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("command provider: bad JSON: %w", err)
	}
	return items, nil
}

// cmdComment is a comment as the detail command prints it.
type cmdComment struct {
	Author  string    `json:"author"`
	Body    string    `json:"body"`
	Created time.Time `json:"created"` // RFC 3339; optional
}

// cmdDetail is the JSON the detail command prints; durations are in seconds.
type cmdDetail struct {
	Description string `json:"description"`
	Jobs        []struct {
		Name     string  `json:"name"`
		Stage    string  `json:"stage"`
		Status   string  `json:"status"`
		Duration float64 `json:"duration"`
	} `json:"jobs"`
	Threads []struct {
		File     string       `json:"file"`
		Line     int          `json:"line"`
		Resolved bool         `json:"resolved"`
		Comments []cmdComment `json:"comments"`
	} `json:"threads"`
	Comments []cmdComment `json:"comments"`
}

// Detail runs DetailCmd with the item's id in $LANES_ITEM_ID; ErrNoDetail when none is configured.
func (c *Command) Detail(ctx context.Context, it item.Item) (item.Detail, error) {
	if c.DetailCmd == "" {
		return item.Detail{}, ErrNoDetail
	}
	out, err := runShell(ctx, c.DetailCmd, "LANES_ITEM_ID="+it.ID)
	if err != nil {
		return item.Detail{}, err
	}
	var r cmdDetail
	if err := json.Unmarshal(out, &r); err != nil {
		return item.Detail{}, fmt.Errorf("command provider: bad JSON: %w", err)
	}
	comments := func(cs []cmdComment) []item.Comment {
		var out []item.Comment
		for _, c := range cs {
			out = append(out, item.Comment(c))
		}
		return out
	}
	d := item.Detail{Description: r.Description, Comments: comments(r.Comments)}
	if len(r.Jobs) > 0 {
		d.Pipeline = &item.Pipeline{}
		for _, j := range r.Jobs {
			d.Pipeline.Jobs = append(d.Pipeline.Jobs, item.Job{Name: j.Name, Stage: j.Stage, Status: j.Status,
				Duration: time.Duration(j.Duration * float64(time.Second))})
		}
	}
	for _, t := range r.Threads {
		d.Threads = append(d.Threads, item.Thread{File: t.File, Line: t.Line, Resolved: t.Resolved, Comments: comments(t.Comments)})
	}
	return d, nil
}

// runShell runs cmd through $SHELL -c (/bin/sh when unset) with env added to the environment and returns its stdout;
// a failure carries its stderr.
func runShell(ctx context.Context, cmd string, env ...string) ([]byte, error) {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	x := exec.CommandContext(ctx, sh, "-c", cmd)
	if len(env) > 0 {
		x.Env = append(os.Environ(), env...)
	}
	out, err := x.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("command provider: %w: %s", err, bytes.TrimSpace(ee.Stderr))
		}
		return nil, fmt.Errorf("command provider: %w", err)
	}
	return out, nil
}
