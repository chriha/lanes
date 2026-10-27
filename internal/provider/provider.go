// Package provider produces items from a source (GitLab, GitHub, Linear, arbitrary command).
package provider

import (
	"context"
	"fmt"
	"strings"

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
	case "auto":
		u, err := origin()
		if err != nil {
			return nil, fmt.Errorf("auto: cannot detect the forge without a git remote origin; set type = \"github\" or \"gitlab\"")
		}
		typ, repo := Forge(u)
		c.Type = typ
		if c.Project == "" {
			c.Project = repo
		}
		return newProvider(c, origin)
	case "gitlab":
		return &GitLab{Project: c.Project}, nil
	case "github":
		return &GitHub{Project: c.Project}, nil
	case "github-issues":
		return &GitHubIssues{Project: c.Project}, nil
	case "linear":
		return &Linear{Team: c.Team, TokenEnv: c.TokenEnv}, nil
	case "command":
		return &Command{Cmd: c.Command}, nil
	}
	return nil, fmt.Errorf("unknown provider type %q", c.Type)
}

// Forge returns the provider type and the repo path (owner/repo, group/project) for a git remote URL
// (https, ssh:// or scp-like git@host:path). Only github.com is github; any other host is gitlab.
func Forge(remoteURL string) (typ, repo string) {
	host, repo := splitRemote(remoteURL)
	if strings.EqualFold(host, "github.com") {
		return "github", repo
	}
	return "gitlab", repo
}

// splitRemote splits a git remote URL into its host (without user and port) and repo path.
func splitRemote(u string) (host, path string) {
	u = strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(u), "/"), ".git")
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
		host, path, _ = strings.Cut(u, "/")
		host, _, _ = strings.Cut(host, ":")
	} else if h, p, ok := strings.Cut(u, ":"); ok {
		host, path = h, p
	} else {
		path = u
	}
	if i := strings.LastIndex(host, "@"); i >= 0 {
		host = host[i+1:]
	}
	return host, path
}

// OriginForge returns the provider type and repo path of the current directory's origin remote
// (see Forge), or an error when there is no origin.
func OriginForge() (typ, repo string, err error) {
	return "", "", fmt.Errorf("not implemented")
}
