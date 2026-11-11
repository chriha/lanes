package provider

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/item"
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
			calls := 0
			origin := func() (string, error) { calls++; return tt.origin() }
			built, err := newProvider(tt.cfg, origin)
			if err != nil {
				t.Fatalf("building an auto provider failed: %v", err)
			}
			if calls != 0 {
				t.Fatal("building an auto provider read the origin; it must wait for the first fetch")
			}
			a, ok := built.(*auto)
			if !ok {
				t.Fatalf("got %T, want *auto", built)
			}
			p, err := a.resolve()
			if _, err2 := a.resolve(); calls != 1 || (err == nil) != (err2 == nil) {
				t.Errorf("origin read %d times over two resolves (errs %v, %v), want once", calls, err, err2)
			}
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

func TestAutoFetchReportsMissingOrigin(t *testing.T) {
	p, err := newProvider(config.Provider{Type: "auto"}, func() (string, error) { return "", errors.New("exit status 2") })
	if err != nil {
		t.Fatal(err)
	}
	const want = `set type = "github" or "gitlab"`
	if _, err := p.Fetch(context.Background()); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("Fetch err = %v, want it to contain %q", err, want)
	}
	q, ok := p.(Quicker)
	if !ok {
		t.Fatal("auto provider has no Quick pass")
	}
	if _, err := q.Quick(context.Background()); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("Quick err = %v, want it to contain %q", err, want)
	}
}

func TestAutoQuickWithoutQuickPassFails(t *testing.T) {
	// github has no Quick pass: auto's Quick must fail rather than return an empty partial result
	p, err := newProvider(config.Provider{Type: "auto"}, func() (string, error) { return "git@github.com:o/r.git", nil })
	if err != nil {
		t.Fatal(err)
	}
	items, err := p.(Quicker).Quick(context.Background())
	if err == nil || items != nil {
		t.Errorf("Quick = %v, %v; want no items and an error", items, err)
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

func TestAutoDetail(t *testing.T) {
	tests := []struct {
		name   string
		origin func() (string, error)
		want   string
	}{
		{"forge without details", func() (string, error) { return "git@github.com:o/r.git", nil }, ErrNoDetail.Error()},
		{"missing origin", func() (string, error) { return "", errors.New("exit status 2") }, `set type = "github" or "gitlab"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := newProvider(config.Provider{Type: "auto"}, tt.origin)
			if err != nil {
				t.Fatal(err)
			}
			d, ok := p.(Detailer)
			if !ok {
				t.Fatal("auto provider has no Detail")
			}
			if _, err := d.Detail(context.Background(), item.Item{ID: "1"}); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Detail err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}
