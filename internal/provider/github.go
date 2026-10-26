package provider

import (
	"context"
	"errors"

	"github.com/chriha/lanes/internal/item"
)

// GitHub lists open PRs through `gh api graphql`, which reuses gh's auth.
type GitHub struct {
	Project string // owner/repo; detected from the origin remote when empty
	// Run executes gh with args; nil runs the real gh binary.
	Run func(ctx context.Context, args ...string) ([]byte, error)
}

func (g *GitHub) Fetch(ctx context.Context) ([]item.Item, error) {
	return nil, errors.New("github: not implemented")
}
