package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/chriha/lanes/internal/item"
)

// Command runs a shell command that prints a JSON array of items.
type Command struct{ Cmd string }

func (c *Command) Fetch(ctx context.Context) ([]item.Item, error) {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	out, err := exec.CommandContext(ctx, sh, "-c", c.Cmd).Output()
	if err != nil {
		return nil, fmt.Errorf("command provider: %w", err)
	}
	var items []item.Item
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("command provider: bad JSON: %w", err)
	}
	return items, nil
}
