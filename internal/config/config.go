// Package config loads the TOML config, writing a default on first run.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/filter"
	"github.com/chriha/lanes/internal/item"
)

type Settings struct {
	PollInterval string `toml:"poll_interval"`
	Preview      bool   `toml:"preview"` // show the detail pane at startup (toggle with p)
	NoNotify     bool   `toml:"no_notify"`
	ClaudeDir    string `toml:"claude_dir"` // Claude config dir whose usage the footer shows, e.g. ~/.claude
}

// Theme overrides the default adaptive palette; values are hex colors like "#A78BFA".
type Theme struct {
	Accent string `toml:"accent"`
	OK     string `toml:"ok"`
	Warn   string `toml:"warn"`
	Bad    string `toml:"bad"`
	Info   string `toml:"info"`
}

type Provider struct {
	Type     string `toml:"type"`      // gitlab | github | github-issues | linear | auto (github for a github.com origin, else gitlab) | command
	Project  string `toml:"project"`   // gitlab: group/project, github: owner/repo; empty = detect from git remote
	Command  string `toml:"command"`   // command: shell command printing a JSON array of items
	Team     string `toml:"team"`      // linear: team key, e.g. ENG (required)
	TokenEnv string `toml:"token_env"` // linear: env var holding the API key; empty = LINEAR_API_KEY
}

type View struct {
	Name     string `toml:"name"`
	Provider string `toml:"provider"`
	Filter   string `toml:"filter"`
	Sort     string `toml:"sort"` // comma-separated fields, "-" prefix = descending; empty = provider order
}

type Action struct {
	Type    string         `toml:"type"`    // session (default): tmux session you can jump into | run: headless, result shown in the footer
	Run     string         `toml:"run"`     // templated shell command, e.g. claude --settings {{.settings}} "/x {{.iid}}"
	Cwd     string         `toml:"cwd"`     // templated working dir; empty = current dir
	Confirm string         `toml:"confirm"` // none (default) | single; multi-item runs always confirm
	Vars    map[string]any `toml:"vars"`    // available in templates as {{.vars.name}}
	Refresh bool           `toml:"refresh"` // run type: refetch providers when finished
	When    string         `toml:"when"`    // expr against the item's fields; the action only runs on matching items (empty = all)
	Jump    bool           `toml:"jump"`    // session type: on a single item, attach to the session once started (or if running)
}

// ShellAction is the name of the built-in shell session (key t); config actions cannot use it.
const ShellAction = "shell"

type Config struct {
	Settings Settings            `toml:"settings"`
	Provider map[string]Provider `toml:"provider"`
	View     []View              `toml:"view"`
	Keymap   map[string]string   `toml:"keymap"`
	Action   map[string]Action   `toml:"action"`
	Theme    Theme               `toml:"theme"`
}

// ProjectFile is the per-project config, found in the working directory or the nearest parent
// (the search stops at the home directory).
const ProjectFile = ".lanes.toml"

func Path() string { return filepath.Join(app.ConfigDir(), "config.toml") }

// ProjectPath returns the nearest project config, or "" when there is none.
func ProjectPath() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	home, _ := os.UserHomeDir()
	for {
		p := filepath.Join(dir, ProjectFile)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir || dir == home {
			return ""
		}
		dir = parent
	}
}

// Mtime is the newest modification time of the config files in use (0 if none exist).
func Mtime() time.Time {
	var t time.Time
	for _, p := range []string{Path(), ProjectPath()} {
		if st, err := os.Stat(p); err == nil && st.ModTime().After(t) {
			t = st.ModTime()
		}
	}
	return t
}

func mergeViews(global, project []View) []View {
	out := append([]View{}, global...)
	for _, pv := range project {
		replaced := false
		for i := range out {
			if out[i].Name == pv.Name {
				out[i], replaced = pv, true
			}
		}
		if !replaced {
			out = append(out, pv)
		}
	}
	return out
}

func (c Config) Poll() time.Duration {
	d, err := time.ParseDuration(c.Settings.PollInterval)
	if err != nil || d < 5*time.Second {
		return 30 * time.Second
	}
	return d
}

// Load reads the global config (writing a default on first run) and overlays the project config:
// settings and theme merge per field, keymap per key, providers/actions/views per name.
func Load() (*Config, error) {
	p := Path()
	if _, err := os.Stat(p); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(p, []byte(Default), 0o644); err != nil {
			return nil, err
		}
	}
	var c Config
	if _, err := toml.DecodeFile(p, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	if pp := ProjectPath(); pp != "" {
		var pc Config
		if _, err := toml.DecodeFile(pp, &pc); err != nil {
			return nil, fmt.Errorf("%s: %w", pp, err)
		}
		global := append([]View(nil), c.View...) // copy: decoding below reuses the backing array
		// decoding over c only touches the keys the file defines; slices are replaced, so views merge by hand
		if _, err := toml.DecodeFile(pp, &c); err != nil {
			return nil, fmt.Errorf("%s: %w", pp, err)
		}
		c.View = mergeViews(global, pc.View)
		p = pp + " (project) over " + p
	}
	if len(c.View) == 0 {
		return nil, fmt.Errorf("%s: no [[view]] defined", p)
	}
	for _, v := range c.View {
		if _, ok := c.Provider[v.Provider]; !ok {
			return nil, fmt.Errorf("%s: view %q uses unknown provider %q", p, v.Name, v.Provider)
		}
		if _, err := item.ParseSort(v.Sort); err != nil {
			return nil, fmt.Errorf("%s: view %q: %w", p, v.Name, err)
		}
	}
	for name, a := range c.Action {
		if strings.Contains(name, "~") || a.Run == "" || (a.Type != "" && a.Type != "session" && a.Type != "run") {
			return nil, fmt.Errorf("%s: action %q needs a run command, type session|run, and no ~ in its name", p, name)
		}
		if name == ShellAction {
			return nil, fmt.Errorf("%s: action %q: the name is reserved for the built-in shell (t); rename the action", p, name)
		}
		if a.Jump && a.Type == "run" {
			return nil, fmt.Errorf("%s: action %q: jump needs type session", p, name)
		}
		if _, err := filter.Compile(a.When); err != nil {
			return nil, fmt.Errorf("%s: action %q: when: %w", p, name, err)
		}
	}
	for key, name := range c.Keymap {
		if _, ok := c.Action[name]; !ok {
			return nil, fmt.Errorf("%s: keymap %q -> unknown action %q", p, key, name)
		}
	}
	return &c, nil
}

const Default = `# lanes — global config. A ".lanes.toml" in a project overlays this file (see README).

[settings]
poll_interval = "30s"
# preview = true      # show the detail pane at startup (toggle with p)
# no_notify = true    # disable desktop notifications
# claude_dir = "~/.claude"  # show this Claude profile's usage in the footer (written by a Claude Code mod)

# type "auto" picks github for a github.com origin and gitlab otherwise, per project
# (GitHub Enterprise: set type = "github"). project is detected from the git remote of the
# current directory when empty.
[provider.forge]
type = "auto"
# project = "group/project"

# Filter fields (gitlab, github): author, draft, approved, approved_by_me, open_threads,
# open_thread_by_me, ci, conflicts, diverged, labels, reviewers, assignees, branch, target, ticket, me
[[view]]
name = "review"
provider = "forge"
filter = '!draft && !approved_by_me && !open_thread_by_me && author != me'

[[view]]
name = "mine"
provider = "forge"
filter = 'author == me && !draft'

[[view]]
name = "drafts"
provider = "forge"
filter = 'author == me && draft'

# Keys run an action on the marked items (or the item under the cursor).
# Built-in keys win over the keymap.
# Template fields: id, title, url, author, plus provider fields (gitlab: iid, github: number,
# branch, ticket, ...).
# {{.x | q}} shell-quotes a value; {{.settings}} lets claude report its status to lanes.
[keymap]
m = "ready"

# type = "run" executes headless (no tmux session) and reports the result in the footer.
# GitLab only; on GitHub use run = 'gh pr ready {{.number}}'.
[action.ready]
type = "run"
refresh = true
run = 'glab mr update {{.iid}} --ready'

# A Claude Code session you can jump into (enter) and that reports working / asking / waiting:
# [keymap]
# r = "review"
# [action.review]
# run = 'claude --settings {{.settings}} "/review {{.url}}"'

# [theme]
# accent = "#A78BFA"
`
