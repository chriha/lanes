package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chriha/lanes/internal/config"
)

func TestDoctor(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := func() *config.Config {
		return &config.Config{
			Provider: map[string]config.Provider{"gitlab": {Type: "gitlab"}},
			View:     []config.View{{Name: "review", Provider: "gitlab", Filter: "!draft"}},
			Action:   map[string]config.Action{"ready": {Type: "run", Run: "glab mr update {{.iid | q}} --ready"}},
		}
	}
	good := func() doctorEnv {
		return doctorEnv{
			lookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil },
			run: func(name string, args ...string) (string, error) {
				if name == "tmux" {
					return "tmux 3.4\n", nil
				}
				return "Logged in to gitlab.com as me\n", nil
			},
			getenv:    func(string) string { return "" },
			load:      func() (*config.Config, error) { return cfg(), nil },
			overrides: func() (*config.Overrides, error) { return &config.Overrides{Filters: map[string]string{}}, nil },
			dirs:      []namedDir{{"state dir", filepath.Join(dir, "state")}, {"cache dir", filepath.Join(dir, "cache")}},
		}
	}
	notFound := func(string) (string, error) { return "", errors.New("not found") }
	missing := func(cli string) func(string) (string, error) {
		return func(n string) (string, error) {
			if n == cli {
				return "", errors.New("not found")
			}
			return "/usr/bin/" + n, nil
		}
	}
	withProvider := func(p config.Provider) func() (*config.Config, error) {
		return func() (*config.Config, error) {
			c := cfg()
			c.Provider = map[string]config.Provider{"gitlab": p}
			return c, nil
		}
	}
	withOrigin := func(url string, err error) func(string, ...string) (string, error) {
		return func(name string, _ ...string) (string, error) {
			switch name {
			case "tmux":
				return "tmux 3.4\n", nil
			case "git":
				return url, err
			}
			return "Logged in as me\n", nil
		}
	}

	tests := []struct {
		name  string
		edit  func(e *doctorEnv)
		code  int
		want  []string // substrings of the output
		avoid []string
	}{
		{name: "all pass", edit: func(*doctorEnv) {}, code: 0,
			want:  []string{"ok    tmux: tmux 3.4", "ok    glab: logged in", "ok    view review", "ok    action ready", "ok    state dir", "ok    cache dir"},
			avoid: []string{"fail", "warn"}},
		{name: "tmux missing", code: 1, want: []string{"fail  tmux"},
			edit: func(e *doctorEnv) {
				e.lookPath = func(n string) (string, error) {
					if n == "tmux" {
						return "", errors.New("not found")
					}
					return "/usr/bin/" + n, nil
				}
			}},
		{name: "old tmux warns", code: 0, want: []string{"warn  tmux"},
			edit: func(e *doctorEnv) {
				e.run = func(string, ...string) (string, error) { return "tmux 3.1c\n", nil }
			}},
		{name: "glab missing with gitlab provider", code: 1, want: []string{"fail  glab"},
			edit: func(e *doctorEnv) { e.lookPath = notFound }},
		{name: "glab not logged in", code: 1, want: []string{"fail  glab auth"},
			edit: func(e *doctorEnv) {
				e.run = func(name string, _ ...string) (string, error) {
					if name == "glab" {
						return "not logged in\n", errors.New("exit status 1")
					}
					return "tmux 3.4\n", nil
				}
			}},
		{name: "no gitlab provider skips glab", code: 0, want: []string{"ok    glab: skipped"}, avoid: []string{"fail"},
			edit: func(e *doctorEnv) {
				e.lookPath = func(n string) (string, error) {
					if n == "glab" {
						return "", errors.New("not found")
					}
					return "/usr/bin/" + n, nil
				}
				e.load = func() (*config.Config, error) {
					c := cfg()
					c.Provider = map[string]config.Provider{"gitlab": {Type: "command", Command: "echo []"}}
					return c, nil
				}
			}},
		{name: "github provider checks gh, not glab", code: 1, want: []string{"fail  gh", "ok    glab: skipped"},
			edit: func(e *doctorEnv) {
				e.lookPath = missing("gh")
				e.load = withProvider(config.Provider{Type: "github"})
			}},
		{name: "github-issues provider checks gh, not glab", code: 1, want: []string{"fail  gh", "ok    glab: skipped"},
			edit: func(e *doctorEnv) {
				e.lookPath = missing("gh")
				e.load = withProvider(config.Provider{Type: "github-issues"})
			}},
		{name: "github-issues provider with gh logged in", code: 0, want: []string{"ok    gh: logged in"},
			avoid: []string{"fail", "warn"},
			edit: func(e *doctorEnv) {
				e.lookPath = missing("glab")
				e.load = withProvider(config.Provider{Type: "github-issues"})
			}},
		{name: "linear provider with the token set", code: 0, want: []string{"ok    provider gitlab: LINEAR_API_KEY is set"},
			avoid: []string{"fail", "warn", "lin_api_secret"},
			edit: func(e *doctorEnv) {
				e.load = withProvider(config.Provider{Type: "linear", Team: "ENG"})
				e.getenv = func(k string) string {
					if k == "LINEAR_API_KEY" {
						return "lin_api_secret"
					}
					return ""
				}
			}},
		{name: "linear provider without the token", code: 1, want: []string{"fail  provider gitlab: LINEAR_API_KEY is not set"},
			edit: func(e *doctorEnv) { e.load = withProvider(config.Provider{Type: "linear", Team: "ENG"}) }},
		{name: "linear provider without a team", code: 1, want: []string{"fail  provider gitlab: team is not set"},
			edit: func(e *doctorEnv) {
				e.load = withProvider(config.Provider{Type: "linear"})
				e.getenv = func(string) string { return "lin_api_secret" }
			}},
		{name: "linear provider with a custom token_env", code: 1, want: []string{"fail  provider gitlab: MY_KEY is not set"},
			edit: func(e *doctorEnv) {
				e.load = withProvider(config.Provider{Type: "linear", Team: "ENG", TokenEnv: "MY_KEY"})
				e.getenv = func(k string) string {
					if k == "LINEAR_API_KEY" {
						return "lin_api_secret"
					}
					return ""
				}
			}},
		{name: "auto on a github origin checks gh", code: 0, want: []string{"ok    gh: logged in", "ok    glab: skipped"},
			avoid: []string{"fail", "warn"},
			edit: func(e *doctorEnv) {
				e.lookPath = missing("glab")
				e.load = withProvider(config.Provider{Type: "auto"})
				e.run = withOrigin("git@github.com:owner/repo.git\n", nil)
			}},
		{name: "auto on a gitlab origin checks glab", code: 0, want: []string{"ok    glab: logged in", "ok    gh: skipped"},
			avoid: []string{"fail", "warn"},
			edit: func(e *doctorEnv) {
				e.lookPath = missing("gh")
				e.load = withProvider(config.Provider{Type: "auto"})
				e.run = withOrigin("https://gitlab.com/group/project.git\n", nil)
			}},
		{name: "auto without an origin warns", code: 0, want: []string{"warn  provider gitlab: auto"},
			edit: func(e *doctorEnv) {
				e.load = withProvider(config.Provider{Type: "auto"})
				e.run = withOrigin("fatal: not a git repository\n", errors.New("exit status 128"))
			}},
		{name: "config fails", code: 1, want: []string{"fail  config: boom"},
			edit: func(e *doctorEnv) { e.load = func() (*config.Config, error) { return nil, errors.New("boom") } }},
		{name: "bad filter", code: 1, want: []string{"fail  view review"},
			edit: func(e *doctorEnv) {
				e.load = func() (*config.Config, error) {
					c := cfg()
					c.View[0].Filter = "draft &&"
					return c, nil
				}
			}},
		{name: "bad override filter", code: 1, want: []string{"fail  view review"},
			edit: func(e *doctorEnv) {
				e.overrides = func() (*config.Overrides, error) {
					return &config.Overrides{Filters: map[string]string{"review": "(("}}, nil
				}
			}},
		{name: "corrupt overrides file", code: 1, want: []string{"fail  overrides"},
			edit: func(e *doctorEnv) {
				e.overrides = func() (*config.Overrides, error) {
					return &config.Overrides{Filters: map[string]string{}}, errors.New("views.json: invalid JSON")
				}
			}},
		{name: "bad override view", code: 1, want: []string{"fail  view extra"},
			edit: func(e *doctorEnv) {
				e.overrides = func() (*config.Overrides, error) {
					return &config.Overrides{Filters: map[string]string{}, Views: []config.View{{Name: "extra", Provider: "gitlab", Filter: "(("}}}, nil
				}
			}},
		{name: "bad action template", code: 1, want: []string{"fail  action ready"},
			edit: func(e *doctorEnv) {
				e.load = func() (*config.Config, error) {
					c := cfg()
					c.Action["ready"] = config.Action{Run: "echo {{.iid"}
					return c, nil
				}
			}},
		{name: "bad action cwd template", code: 1, want: []string{"fail  action ready"},
			edit: func(e *doctorEnv) {
				e.load = func() (*config.Config, error) {
					c := cfg()
					c.Action["ready"] = config.Action{Run: "true", Cwd: "{{.nope | nofunc}}"}
					return c, nil
				}
			}},
		{name: "dir not writable", code: 1, want: []string{"fail  state dir"},
			edit: func(e *doctorEnv) { e.dirs[0].path = filepath.Join(file, "state") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := good()
			tt.edit(&e)
			var b bytes.Buffer
			code := runDoctor(&b, e)
			out := b.String()
			if code != tt.code {
				t.Errorf("exit %d, want %d\n%s", code, tt.code, out)
			}
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("output lacks %q\n%s", w, out)
				}
			}
			for _, a := range tt.avoid {
				if strings.Contains(out, a) {
					t.Errorf("output has %q\n%s", a, out)
				}
			}
		})
	}
}

func TestDoctorBadThemeColor(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "cfg"))
	t.Setenv("HOME", tmp)
	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	cfgDir := filepath.Join(tmp, "cfg", "lanes")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "[provider.gl]\ntype = \"gitlab\"\n[[view]]\nname = \"a\"\nprovider = \"gl\"\n[theme]\naccent = \"A78BFA\"\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	e := doctorEnv{
		lookPath:  func(name string) (string, error) { return "/usr/bin/" + name, nil },
		run:       func(string, ...string) (string, error) { return "tmux 3.4\n", nil },
		getenv:    func(string) string { return "" },
		load:      config.Load,
		overrides: func() (*config.Overrides, error) { return &config.Overrides{Filters: map[string]string{}}, nil },
	}
	var b bytes.Buffer
	if code := runDoctor(&b, e); code != 1 {
		t.Errorf("exit %d, want 1\n%s", code, b.String())
	}
	if want := `theme.accent "A78BFA"`; !strings.Contains(b.String(), "fail  config:") || !strings.Contains(b.String(), want) {
		t.Errorf("output lacks a failing config line with %q\n%s", want, b.String())
	}
}
