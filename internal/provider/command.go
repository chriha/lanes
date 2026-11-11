package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/chriha/lanes/internal/item"
)

// Command runs a shell command that prints a JSON array of items, and DetailCmd, when set, for an item's detail.
type Command struct{ Cmd, DetailCmd string }

func (c *Command) Fetch(ctx context.Context) ([]item.Item, error) {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	out, err := exec.CommandContext(ctx, sh, "-c", c.Cmd).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("command provider: %w: %s", err, bytes.TrimSpace(ee.Stderr))
		}
		return nil, fmt.Errorf("command provider: %w", err)
	}
	var items []item.Item
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("command provider: bad JSON: %w", err)
	}
	return items, nil
}

// Detail runs DetailCmd for the item; ErrNoDetail when none is configured.
func (c *Command) Detail(ctx context.Context, it item.Item) (item.Detail, error) {
	return item.Detail{}, ErrNoDetail
}
