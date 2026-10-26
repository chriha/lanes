// Package provider produces items from a source (GitLab, arbitrary command).
package provider

import (
	"context"
	"fmt"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/item"
)

type Provider interface {
	Fetch(ctx context.Context) ([]item.Item, error)
}

// Quicker is optionally implemented by providers that can return a fast partial result before the full one.
type Quicker interface {
	Quick(ctx context.Context) ([]item.Item, error)
}

func New(c config.Provider) (Provider, error) {
	switch c.Type {
	case "gitlab":
		return &GitLab{Project: c.Project}, nil
	case "github":
		return &GitHub{Project: c.Project}, nil
	case "command":
		return &Command{Cmd: c.Command}, nil
	}
	return nil, fmt.Errorf("unknown provider type %q", c.Type)
}
