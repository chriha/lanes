package provider

import (
	"errors"
	"strings"
	"testing"

	"github.com/chriha/lanes/internal/config"
)

func TestForge(t *testing.T) {
	tests := []struct {
		url, typ, repo string
	}{
		{"https://github.com/owner/repo.git", "github", "owner/repo"},
		{"https://github.com/owner/repo", "github", "owner/repo"},
		{"https://github.com/owner/repo.git/", "github", "owner/repo"},
		{"git@github.com:owner/repo.git", "github", "owner/repo"},
		{"ssh://git@github.com/owner/repo.git", "github", "owner/repo"},
		{"ssh://git@github.com:22/owner/repo.git", "github", "owner/repo"},
		{"https://user@GitHub.com/owner/repo.git", "github", "owner/repo"},
		{"https://gitlab.com/group/sub/project.git", "gitlab", "group/sub/project"},
		{"git@gitlab.example.com:group/project.git", "gitlab", "group/project"},
		{"https://github.example.com/owner/repo.git", "gitlab", "owner/repo"}, // Enterprise needs type = "github"
		{"https://notgithub.com/owner/repo.git", "gitlab", "owner/repo"},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			typ, repo := Forge(tt.url)
			if typ != tt.typ || repo != tt.repo {
				t.Errorf("Forge(%q) = %q, %q, want %q, %q", tt.url, typ, repo, tt.typ, tt.repo)
			}
		})
	}
}

func TestNewAuto(t *testing.T) {
	origin := func(u string, err error) func() (string, error) {
		return func() (string, error) { return u, err }
	}
	tests := []struct {
		name    string
		cfg     config.Provider
		origin  func() (string, error)
		want    Provider
		wantErr string
	}{
		{name: "github origin", cfg: config.Provider{Type: "auto"},
			origin: origin("git@github.com:owner/repo.git", nil), want: &GitHub{Project: "owner/repo"}},
		{name: "gitlab origin", cfg: config.Provider{Type: "auto"},
			origin: origin("https://gitlab.com/group/project.git", nil), want: &GitLab{Project: "group/project"}},
		{name: "explicit project wins", cfg: config.Provider{Type: "auto", Project: "other/repo"},
			origin: origin("https://github.com/owner/repo", nil), want: &GitHub{Project: "other/repo"}},
		{name: "missing origin", cfg: config.Provider{Type: "auto"},
			origin: origin("", errors.New("exit status 2")), wantErr: `set type = "github" or "gitlab"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := newProvider(tt.cfg, tt.origin)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			switch w := tt.want.(type) {
			case *GitHub:
				g, ok := p.(*GitHub)
				if !ok || g.Project != w.Project {
					t.Errorf("got %#v, want github provider for %q", p, w.Project)
				}
			case *GitLab:
				g, ok := p.(*GitLab)
				if !ok || g.Project != w.Project {
					t.Errorf("got %#v, want gitlab provider for %q", p, w.Project)
				}
			}
		})
	}
}

func TestNewExplicitTypeIgnoresOrigin(t *testing.T) {
	called := false
	origin := func() (string, error) { called = true; return "", errors.New("no origin") }
	if _, err := newProvider(config.Provider{Type: "github"}, origin); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("origin read for an explicit provider type")
	}
}
