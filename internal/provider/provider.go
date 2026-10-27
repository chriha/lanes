// Package provider produces items from a source (GitLab, GitHub, arbitrary command).
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
	return newProvider(c, originURL)
}

// newProvider builds c; origin returns the origin remote's URL, read only for type "auto".
func newProvider(c config.Provider, origin func() (string, error)) (Provider, error) {
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

// Forge returns the provider type ("github" or "gitlab") and the repo path for a git remote URL.
func Forge(remoteURL string) (typ, repo string) {
	return "", ""
}
