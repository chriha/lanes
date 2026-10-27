package provider

import (
	"context"
	"errors"

	"github.com/chriha/lanes/internal/item"
)

// GitHubIssues lists a repository's open issues as tickets through `gh api graphql`.
type GitHubIssues struct {
	Project string // owner/repo; detected from the origin remote when empty
	// Run executes gh with args; nil runs the real gh binary.
	Run func(ctx context.Context, args ...string) ([]byte, error)
}

func (g *GitHubIssues) Fetch(ctx context.Context) ([]item.Item, error) {
	return nil, errors.New("github-issues: not implemented")
}
