// Package config loads the TOML config, writing a default on first run.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"text/template/parse"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/filter"
	"github.com/chriha/lanes/internal/item"
)

type Settings struct {
	PollInterval string `toml:"poll_interval"`
	PollBlurred  string `toml:"poll_interval_blurred"` // poll interval while the terminal is unfocused
	Preview      bool   `toml:"preview"`               // show the detail pane at startup (toggle with p)
	NoNotify     bool   `toml:"no_notify"`
	ClaudeDir    string `toml:"claude_dir"` // Claude config dir whose usage the footer shows, e.g. ~/.claude
}

// Colors are hex colors like "#A78BFA" overriding the default adaptive palette.
type Colors struct {
	Accent string `toml:"accent"`
	OK     string `toml:"ok"`
	Warn   string `toml:"warn"`
	Bad    string `toml:"bad"`
	Info   string `toml:"info"`

	Text      string `toml:"text"`
	Dim       string `toml:"dim"`
	Selection string `toml:"selection"`

	Labels map[string]string `toml:"labels"` // label name (case-insensitive) -> hex color, for the preview's labels line
}

// Theme holds the [theme] table: the flat color keys apply to both modes, [theme.light] / [theme.dark] to one.
type Theme struct {
	Colors
	Mode  string `toml:"mode"` // system (default, the terminal background) | light | dark
	Light Colors `toml:"light"`
	Dark  Colors `toml:"dark"`
}

var hexColor = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// validate rejects any non-empty color that is not #RGB or #RRGGBB; it walks the string fields, so new keys are covered.
func (c Colors) validate(prefix string) error {
	v := reflect.ValueOf(c)
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).Kind() != reflect.String {
			continue
		}
		if s := v.Field(i).String(); s != "" && !hexColor.MatchString(s) {
			return fmt.Errorf("%s%s %q: want #RGB or #RRGGBB", prefix, v.Type().Field(i).Tag.Get("toml"), s)
		}
	}
	return validateLabels(c.Labels, prefix+"labels.")
}

// validateLabels rejects a label color that is not #RGB or #RRGGBB, and two names that differ only in case.
func validateLabels(labels map[string]string, prefix string) error {
	names := make([]string, 0, len(labels))
	for name := range labels {
		names = append(names, name)
	}
	sort.Strings(names) // deterministic errors
	seen := map[string]string{}
	for _, name := range names {
		if s := labels[name]; !hexColor.MatchString(s) {
			return fmt.Errorf("%s%s %q: want #RGB or #RRGGBB", prefix, name, s)
		}
		lower := strings.ToLower(name)
		if prev, ok := seen[lower]; ok {
			return fmt.Errorf("%s%s: same label as %s%s (names match case-insensitively)", prefix, name, prefix, prev)
		}
		seen[lower] = name
	}
	return nil
}

// dropShadowedLabels removes the label keys of merged that a project label key overrides by differing only in case,
// so the project's spelling replaces the global one instead of both colliding in validateLabels.
func dropShadowedLabels(merged *Colors, project Colors) {
	for name := range merged.Labels {
		if _, ok := project.Labels[name]; ok {
			continue
		}
		for pname := range project.Labels {
			if strings.EqualFold(name, pname) {
				delete(merged.Labels, name)
				break
			}
		}
	}
}

func (t Theme) validate() error {
	switch t.Mode {
	case "", "system", "light", "dark":
	default:
		return fmt.Errorf("theme.mode %q: want system, light or dark", t.Mode)
	}
	if err := t.Colors.validate("theme."); err != nil {
		return err
	}
	if err := t.Light.validate("theme.light."); err != nil {
		return err
	}
	return t.Dark.validate("theme.dark.")
}

type Provider struct {
	Type     string `toml:"type"`      // gitlab | github | github-issues | linear | auto (github for a github.com origin, else gitlab) | command
	Project  string `toml:"project"`   // gitlab: group/project, github: owner/repo; empty = detect from git remote
	Command  string `toml:"command"`   // command: shell command printing a JSON array of items
	Team     string `toml:"team"`      // linear: team key, e.g. ENG (required)
	TokenEnv string `toml:"token_env"` // linear: env var holding the API key; empty = LINEAR_API_KEY
}

type View struct {
	Name     string  `toml:"name"`
	Provider string  `toml:"provider"`
	Filter   string  `toml:"filter"`
	Sort     string  `toml:"sort"`   // comma-separated fields, "-" prefix = descending; empty = provider order
	Groups   []Group `toml:"groups"` // named filters; an item goes into the first group it matches, the rest stay ungrouped
}

// Group is a named filter inside a view.
type Group struct {
	Name   string `toml:"name"`
	Filter string `toml:"filter"`
}

type Action struct {
	Type    string         `toml:"type"`    // session (default): tmux session you can jump into | run: headless, result shown in the footer
	Run     string         `toml:"run"`     // templated shell command, e.g. claude --settings {{.settings}} "/x {{.iid}}"
	Resume  string         `toml:"resume"`  // optional; templated like run, plus {{.claude_session}}: resumes a finished Claude session (gr)
	Cwd     string         `toml:"cwd"`     // templated working dir; empty = current dir
	Confirm string         `toml:"confirm"` // none (default) | single; multi-item runs always confirm
	Vars    map[string]any `toml:"vars"`    // available in templates as {{.vars.name}}
	Refresh bool           `toml:"refresh"` // run type: refetch providers when finished
	When    string         `toml:"when"`    // expr against the item's fields; the action only runs on matching items (empty = all)
	Jump    bool           `toml:"jump"`    // session type: on a single item, attach to the session once started (or if running)

	WhenFilter *filter.Filter `toml:"-"` // compiled When; filled by CompileWhen (config load), nil on a hand-built action
}

// CompileWhen compiles When into WhenFilter; an empty When yields a filter matching everything. Config load calls it
// for every action, so a hand-built action must too (a nil WhenFilter matches everything).
func (a *Action) CompileWhen() error {
	f, err := filter.Compile(a.When)
	if err != nil {
		return err
	}
	a.WhenFilter = f
	return nil
}

// ClaudeSessionField is the template field holding the Claude session id to resume; only resume templates define it.
const ClaudeSessionField = "claude_session"

// usesField reports whether template text refers to the top-level field name ({{.name}}, {{$.name}}). Inside
// with/range the dot is rebound, but a field of that name is still taken as a use. Text that does not parse reports
// false: doctor and running the action report the parse error with the template functions the UI defines.
func usesField(text, name string) bool {
	tree := parse.New("")
	tree.Mode = parse.SkipFuncCheck
	if _, err := tree.Parse(text, "", "", map[string]*parse.Tree{}); err != nil || tree.Root == nil {
		return false
	}
	return nodeUsesField(tree.Root, name)
}

func nodeUsesField(n parse.Node, name string) bool {
	switch n := n.(type) {
	case *parse.FieldNode:
		return len(n.Ident) > 0 && n.Ident[0] == name
	case *parse.VariableNode:
		return len(n.Ident) > 1 && n.Ident[0] == "$" && n.Ident[1] == name
	case *parse.ChainNode:
		return nodeUsesField(n.Node, name)
	case *parse.ListNode:
		for _, c := range n.Nodes {
			if nodeUsesField(c, name) {
				return true
			}
		}
	case *parse.ActionNode:
		return nodeUsesField(n.Pipe, name)
	case *parse.PipeNode:
		for _, c := range n.Cmds {
			if nodeUsesField(c, name) {
				return true
			}
		}
	case *parse.CommandNode:
		for _, a := range n.Args {
			if nodeUsesField(a, name) {
				return true
			}
		}
	case *parse.IfNode:
		return branchUsesField(&n.BranchNode, name)
	case *parse.RangeNode:
		return branchUsesField(&n.BranchNode, name)
	case *parse.WithNode:
		return branchUsesField(&n.BranchNode, name)
	case *parse.TemplateNode:
		return n.Pipe != nil && nodeUsesField(n.Pipe, name)
	}
	return false
}

func branchUsesField(b *parse.BranchNode, name string) bool {
	return nodeUsesField(b.Pipe, name) ||
		(b.List != nil && nodeUsesField(b.List, name)) ||
		(b.ElseList != nil && nodeUsesField(b.ElseList, name))
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

// PollBlurred is the poll interval while the terminal is unfocused: 5m when unset or invalid, never below Poll().
func (c Config) PollBlurred() time.Duration {
	d, err := time.ParseDuration(c.Settings.PollBlurred)
	if err != nil {
		d = 5 * time.Minute
	}
	return max(d, c.Poll())
}

// Load reads the global config (writing a default on first run) and overlays the project config:
// settings and theme merge per field, keymap per key, providers/actions/views per name.
func Load() (*Config, error) {
	p := Path()
	if _, err := os.Stat(p); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(p), app.DirMode); err != nil {
			return nil, err
		}
		if err := app.WriteFile(p, []byte(Default)); err != nil {
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
		dropShadowedLabels(&c.Theme.Colors, pc.Theme.Colors)
		dropShadowedLabels(&c.Theme.Light, pc.Theme.Light)
		dropShadowedLabels(&c.Theme.Dark, pc.Theme.Dark)
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
		for i, g := range v.Groups {
			if g.Name == "" {
				return nil, fmt.Errorf("%s: view %q: group %d needs a name", p, v.Name, i+1)
			}
			if _, err := filter.Compile(g.Filter); err != nil {
				return nil, fmt.Errorf("%s: view %q: group %q: %w", p, v.Name, g.Name, err)
			}
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
		for _, o := range []struct{ field, text string }{{"run", a.Run}, {"cwd", a.Cwd}} {
			if usesField(o.text, ClaudeSessionField) {
				return nil, fmt.Errorf("%s: action %q: {{.%s}} is only defined in resume, not in %s", p, name, ClaudeSessionField, o.field)
			}
		}
		if err := a.CompileWhen(); err != nil {
			return nil, fmt.Errorf("%s: action %q: when: %w", p, name, err)
		}
		c.Action[name] = a // a is a copy: store the compiled filter
	}
	if err := c.Theme.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
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
# poll_interval_blurred = "5m"  # how often lists refresh while the terminal is unfocused (never below poll_interval)
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
# Values are shell-quoted for you; {{.settings}} lets claude report its status to lanes.
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
# mode = "system"      # system (default): follow the terminal background | light | dark
# accent = "#A78BFA"   # both modes; also: ok, warn, bad, info, text, dim, selection
# [theme.light]        # same color keys, light mode only; they win over the flat [theme] keys
# accent = "#6C3FD1"
# [theme.dark]         # same color keys, dark mode only
# accent = "#A78BFA"
# [theme.labels]       # preview label colors by name, case-insensitive; also [theme.light.labels] / [theme.dark.labels]
# bug = "#F87171"
# needs-info = "#FBBF24"
`
