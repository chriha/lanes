package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProjectOverlay(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "cfg"))
	t.Setenv("HOME", tmp)
	proj := filepath.Join(tmp, "proj", "sub")
	write(t, filepath.Join(tmp, "cfg", "lanes", "config.toml"), `
[settings]
poll_interval = "45s"
preview = true
claude_dir = "~/.claude"
[provider.gl]
type = "gitlab"
[[view]]
name = "a"
provider = "gl"
filter = "true"
[[view]]
name = "b"
provider = "gl"
filter = "false"
[keymap]
r = "x"
s = "x"
[action.x]
run = "echo x"
[theme]
mode = "dark"
accent = "#111111"
ok = "#222222"
text = "#444444"
dim = "#555555"
[theme.light]
accent = "#aaaaaa"
ok = "#bbbbbb"
[theme.dark]
info = "#cccccc"
`)
	write(t, filepath.Join(tmp, "proj", ProjectFile), `
[settings]
preview = false
claude_dir = "~/.claude-personal"
no_update_check = true
[[view]]
name = "b"
provider = "gl"
filter = "draft"
[[view]]
name = "c"
provider = "gl"
[keymap]
s = "y"
[action.y]
run = "echo y"
[theme]
mode = "light"
ok = "#333333"
dim = "#666666"
selection = "#777777"
[theme.light]
ok = "#dddddd"
[theme.dark]
warn = "#eeeeee"
`)
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.Chdir(proj); err != nil {
		t.Fatal(err)
	}

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Settings.PollInterval != "45s" || c.Settings.Preview || c.Settings.ClaudeDir != "~/.claude-personal" || !c.Settings.NoUpdateCheck {
		t.Errorf("settings should merge per field: %+v", c.Settings)
	}
	if got := len(c.View); got != 3 || c.View[1].Filter != "draft" || c.View[2].Name != "c" {
		t.Errorf("views should merge by name: %+v", c.View)
	}
	if c.Keymap["r"] != "x" || c.Keymap["s"] != "y" {
		t.Errorf("keymap should merge per key: %+v", c.Keymap)
	}
	if len(c.Action) != 2 {
		t.Errorf("actions should merge by name: %+v", c.Action)
	}
	if c.Theme.Accent != "#111111" || c.Theme.OK != "#333333" ||
		c.Theme.Text != "#444444" || c.Theme.Dim != "#666666" || c.Theme.Selection != "#777777" {
		t.Errorf("theme should merge per field: %+v", c.Theme)
	}
	if c.Theme.Mode != "light" {
		t.Errorf("theme mode = %q, want the project's %q", c.Theme.Mode, "light")
	}
	if want := (Colors{Accent: "#aaaaaa", OK: "#dddddd"}); !reflect.DeepEqual(c.Theme.Light, want) {
		t.Errorf("[theme.light] should merge per field: %+v, want %+v", c.Theme.Light, want)
	}
	if want := (Colors{Info: "#cccccc", Warn: "#eeeeee"}); !reflect.DeepEqual(c.Theme.Dark, want) {
		t.Errorf("[theme.dark] should merge per field: %+v, want %+v", c.Theme.Dark, want)
	}
}

func TestSortValidation(t *testing.T) {
	tests := []struct {
		sort    string
		wantErr bool
	}{
		{"", false},
		{"-updated_at,title", false},
		{"unknown_field", false},
		{"a,,b", true},
		{"-", true},
	}
	for _, tt := range tests {
		t.Run(tt.sort, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "cfg"))
			t.Setenv("HOME", tmp)
			old, _ := os.Getwd()
			defer os.Chdir(old)
			if err := os.Chdir(tmp); err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(tmp, "cfg", "lanes", "config.toml"), `
[provider.gl]
type = "gitlab"
[[view]]
name = "a"
provider = "gl"
sort = "`+tt.sort+`"
`)
			c, err := Load()
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && c.View[0].Sort != tt.sort {
				t.Errorf("sort = %q, want %q", c.View[0].Sort, tt.sort)
			}
		})
	}
}

func TestFirstRunDefaultUsesAutoForge(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "cfg"))
	t.Setenv("HOME", tmp)
	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Provider["forge"].Type; got != "auto" {
		t.Errorf("provider forge type = %q, want auto (providers: %v)", got, c.Provider)
	}
	for _, v := range c.View {
		if v.Provider != "forge" {
			t.Errorf("view %q uses provider %q, want forge", v.Name, v.Provider)
		}
	}
}

func TestActionWhenValidation(t *testing.T) {
	tests := []struct {
		name    string
		when    string // TOML line for the action, empty = no when key
		wantErr bool
	}{
		{"absent", "", false},
		{"valid", `when = 'kind == "ticket" && !blocked'`, false},
		{"invalid", `when = 'kind ==='`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "cfg"))
			t.Setenv("HOME", tmp)
			old, _ := os.Getwd()
			defer os.Chdir(old)
			if err := os.Chdir(tmp); err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(tmp, "cfg", "lanes", "config.toml"), `
[provider.gl]
type = "gitlab"
[[view]]
name = "a"
provider = "gl"
[action.work]
run = "echo work"
`+tt.when+`
`)
			c, err := Load()
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				if !strings.Contains(err.Error(), `action "work"`) {
					t.Errorf("error %q does not name the action", err)
				}
				return
			}
			want := strings.TrimSuffix(strings.TrimPrefix(tt.when, "when = '"), "'")
			if got := c.Action["work"].When; got != want {
				t.Errorf("when = %q, want %q", got, want)
			}
			if c.Action["work"].WhenFilter == nil {
				t.Error("WhenFilter is nil after load, want the compiled when")
			}
		})
	}
}

func TestActionJumpValidation(t *testing.T) {
	tests := []struct {
		name    string
		action  string // TOML lines for the action
		wantErr bool
	}{
		{"session", "jump = true", false},
		{"explicit session", "type = \"session\"\njump = true", false},
		{"run", "type = \"run\"\njump = true", true},
		{"run without jump", "type = \"run\"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "cfg"))
			t.Setenv("HOME", tmp)
			old, _ := os.Getwd()
			defer os.Chdir(old)
			if err := os.Chdir(tmp); err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(tmp, "cfg", "lanes", "config.toml"), `
[provider.gl]
type = "gitlab"
[[view]]
name = "a"
provider = "gl"
[action.work]
run = "echo work"
`+tt.action+`
`)
			_, err := Load()
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && (!strings.Contains(err.Error(), `action "work"`) || !strings.Contains(err.Error(), "jump")) {
				t.Errorf("error %q does not name the action and jump", err)
			}
		})
	}
}

func TestActionResumeValidation(t *testing.T) {
	tests := []struct {
		name       string
		action     string // TOML lines for the action, after its run line
		run        string
		wantErr    string // substring of the error naming the offending option; "" = loads
		wantResume string
	}{
		{"resume uses claude_session", `resume = 'claude --resume {{.claude_session}}'`, "echo work", "", "claude --resume {{.claude_session}}"},
		{"resume without claude_session", `resume = 'claude --continue'`, "echo work", "", "claude --continue"},
		{"no resume", "", "echo work", "", ""},
		{"resume on a run action", "type = 'run'\nresume = 'claude --continue'", "echo work", "resume needs type session", ""},
		{"resume on a session action", "type = 'session'\nresume = 'claude --continue'", "echo work", "", "claude --continue"},
		{"run uses claude_session", "", "claude --resume {{.claude_session}}", "not in run", ""},
		{"run uses it through $", "", "claude --resume {{$.claude_session}}", "not in run", ""},
		{"run pipes it", "", "claude --resume {{.claude_session | raw}}", "not in run", ""},
		{"run uses it inside if", "", "claude {{if .claude_session}}--resume {{.claude_session}}{{end}}", "not in run", ""},
		{"cwd uses claude_session", "cwd = '/tmp/{{.claude_session}}'", "echo work", "not in cwd", ""},
		{"run uses a var of that name", "", "echo {{.vars.claude_session}}", "", ""},
		{"run mentions it as text", "", "echo claude_session", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "cfg"))
			t.Setenv("HOME", tmp)
			old, _ := os.Getwd()
			defer os.Chdir(old)
			if err := os.Chdir(tmp); err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(tmp, "cfg", "lanes", "config.toml"), `
[provider.gl]
type = "gitlab"
[[view]]
name = "a"
provider = "gl"
[action.work]
run = '`+tt.run+`'
`+tt.action+`
`)
			c, err := Load()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), `action "work"`) || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want one naming action \"work\" and %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want none", err)
			}
			if got := c.Action["work"].Resume; got != tt.wantResume {
				t.Errorf("Resume = %q, want %q", got, tt.wantResume)
			}
		})
	}
}

func TestReservedShellAction(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "cfg"))
	t.Setenv("HOME", tmp)
	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(tmp, "cfg", "lanes", "config.toml"), `
[provider.gl]
type = "gitlab"
[[view]]
name = "a"
provider = "gl"
[action.shell]
run = "echo shell"
`)
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), `action "shell"`) || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("err = %v, want it to say action \"shell\" is reserved", err)
	}
}

func TestViewGroupsValidation(t *testing.T) {
	tests := []struct {
		name    string
		global  string // TOML lines appended to view "a" of the global config
		project string // project .lanes.toml, empty = none
		want    []Group
		wantErr string // substring of the error, empty = no error
	}{
		{"absent", "", "", nil, ""},
		{
			"inline array",
			`groups = [{ name = "mine", filter = 'author == me' }, { name = "urgent", filter = '"urgent" in labels' }]`, "",
			[]Group{{"mine", "author == me"}, {"urgent", `"urgent" in labels`}}, "",
		},
		{
			"array of tables",
			"[[view.groups]]\nname = \"mine\"\nfilter = 'author == me'", "",
			[]Group{{"mine", "author == me"}}, "",
		},
		{"empty filter matches everything", `groups = [{ name = "all" }]`, "", []Group{{"all", ""}}, ""},
		{"missing name", `groups = [{ filter = 'draft' }]`, "", nil, `view "a": group 1 needs a name`},
		{"bad filter", `groups = [{ name = "ok" }, { name = "broken", filter = 'kind ===' }]`, "", nil, `view "a": group "broken"`},
		{
			"project view",
			"",
			"[[view]]\nname = \"a\"\nprovider = \"gl\"\ngroups = [{ name = \"mine\", filter = 'author == me' }]",
			[]Group{{"mine", "author == me"}}, "",
		},
		{
			"project bad filter",
			"",
			"[[view]]\nname = \"p\"\nprovider = \"gl\"\ngroups = [{ name = \"x\", filter = '(' }]",
			nil, `view "p": group "x"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "cfg"))
			t.Setenv("HOME", tmp)
			proj := filepath.Join(tmp, "proj")
			if err := os.MkdirAll(proj, 0o755); err != nil {
				t.Fatal(err)
			}
			old, _ := os.Getwd()
			defer os.Chdir(old)
			if err := os.Chdir(proj); err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(tmp, "cfg", "lanes", "config.toml"), `
[provider.gl]
type = "gitlab"
[[view]]
name = "a"
provider = "gl"
`+tt.global+"\n")
			if tt.project != "" {
				write(t, filepath.Join(proj, ProjectFile), tt.project+"\n")
			}
			c, err := Load()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := c.View[0].Groups; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("groups = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestThemeValidation(t *testing.T) {
	const base = `
[provider.gl]
type = "gitlab"
[[view]]
name = "a"
provider = "gl"
`
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"short hex", "#abc", false},
		{"long hex", "#A78BFA", false},
		{"empty", "", false},
		{"no hash", "A78BFA", true},
		{"not hex digits", "#ZZZ", true},
		{"wrong length", "#12345", true},
		{"color name", "red", true},
		{"ansi number", "212", true},
	}
	fields := reflect.TypeOf(Colors{})
	for _, scope := range []string{"global", "project"} {
		for _, table := range []string{"theme", "theme.light", "theme.dark"} {
			for i := 0; i < fields.NumField(); i++ {
				if fields.Field(i).Type.Kind() != reflect.String {
					continue // the labels table has its own test
				}
				key := table + "." + fields.Field(i).Tag.Get("toml")
				for _, tt := range tests {
					t.Run(scope+"/"+key+"/"+tt.name, func(t *testing.T) {
						tmp := t.TempDir()
						t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "cfg"))
						t.Setenv("HOME", tmp)
						old, _ := os.Getwd()
						defer os.Chdir(old)
						if err := os.Chdir(tmp); err != nil {
							t.Fatal(err)
						}
						theme := "\n[" + table + "]\n" + fields.Field(i).Tag.Get("toml") + " = " + strconv.Quote(tt.value) + "\n"
						if scope == "global" {
							write(t, filepath.Join(tmp, "cfg", "lanes", "config.toml"), base+theme)
						} else {
							write(t, filepath.Join(tmp, "cfg", "lanes", "config.toml"), base)
							write(t, filepath.Join(tmp, ".lanes.toml"), theme)
						}
						_, err := Load()
						if (err != nil) != tt.wantErr {
							t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
						}
						if err != nil && !strings.Contains(err.Error(), key+" "+strconv.Quote(tt.value)) {
							t.Errorf("error %q does not name the key and value", err)
						}
					})
				}
			}
		}
	}
}

func TestThemeModeValidation(t *testing.T) {
	const base = `
[provider.gl]
type = "gitlab"
[[view]]
name = "a"
provider = "gl"
`
	tests := []struct {
		mode    string
		wantErr bool
	}{
		{"", false},
		{"system", false},
		{"light", false},
		{"dark", false},
		{"Dark", true},
		{"auto", true},
		{"#000000", true},
	}
	for _, scope := range []string{"global", "project"} {
		for _, tt := range tests {
			t.Run(scope+"/"+tt.mode, func(t *testing.T) {
				tmp := t.TempDir()
				t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "cfg"))
				t.Setenv("HOME", tmp)
				old, _ := os.Getwd()
				defer os.Chdir(old)
				if err := os.Chdir(tmp); err != nil {
					t.Fatal(err)
				}
				theme := "\n[theme]\nmode = " + strconv.Quote(tt.mode) + "\n"
				if scope == "global" {
					write(t, filepath.Join(tmp, "cfg", "lanes", "config.toml"), base+theme)
				} else {
					write(t, filepath.Join(tmp, "cfg", "lanes", "config.toml"), base)
					write(t, filepath.Join(tmp, ".lanes.toml"), theme)
				}
				c, err := Load()
				if (err != nil) != tt.wantErr {
					t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
				}
				if err != nil {
					if !strings.Contains(err.Error(), "theme.mode "+strconv.Quote(tt.mode)) {
						t.Errorf("error %q does not name the key and value", err)
					}
					return
				}
				if c.Theme.Mode != tt.mode {
					t.Errorf("mode = %q, want %q", c.Theme.Mode, tt.mode)
				}
			})
		}
	}
}

func TestPollBlurred(t *testing.T) {
	tests := []struct {
		name          string
		poll, blurred string
		want          time.Duration
	}{
		{"unset defaults to 5m", "", "", 5 * time.Minute},
		{"valid value is kept", "30s", "2m", 2 * time.Minute},
		{"unparseable falls back to 5m", "30s", "soon", 5 * time.Minute},
		{"below poll_interval is raised to it", "1m", "45s", time.Minute},
		{"zero is raised to poll_interval", "1m", "0s", time.Minute},
		{"equal to poll_interval is kept", "1m", "1m", time.Minute},
		{"default below a long poll_interval is raised to it", "10m", "", 10 * time.Minute},
		{"below the invalid-poll fallback is raised to 30s", "1s", "10s", 30 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Config{Settings: Settings{PollInterval: tt.poll, PollBlurred: tt.blurred}}
			if got := c.PollBlurred(); got != tt.want {
				t.Errorf("PollBlurred() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPollBlurredProjectOverlay(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "cfg"))
	t.Setenv("HOME", tmp)
	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(tmp, "cfg", "lanes", "config.toml"), `
[settings]
poll_interval = "45s"
poll_interval_blurred = "10m"
[provider.gl]
type = "gitlab"
[[view]]
name = "a"
provider = "gl"
`)
	write(t, filepath.Join(tmp, ".lanes.toml"), `
[settings]
poll_interval_blurred = "3m"
`)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Poll(); got != 45*time.Second {
		t.Errorf("Poll() = %v, want 45s kept from the global config", got)
	}
	if got := c.PollBlurred(); got != 3*time.Minute {
		t.Errorf("PollBlurred() = %v, want 3m from the project config", got)
	}
}

func TestThemeLabelsValidation(t *testing.T) {
	const base = `
[provider.gl]
type = "gitlab"
[[view]]
name = "a"
provider = "gl"
`
	tests := []struct {
		name    string
		labels  string // body of the labels table
		wantErr string // substring naming the key; empty = valid
	}{
		{"valid short and long hex", "bug = \"#F87171\"\nneeds-info = \"#abc\"\n", ""},
		{"quoted key with a space", "\"to be deployed\" = \"#4ADE80\"\n", ""},
		{"color name", "bug = \"red\"\n", `labels.bug "red": want #RGB or #RRGGBB`},
		{"no hash", "bug = \"F87171\"\n", `labels.bug "F87171": want #RGB or #RRGGBB`},
		{"empty value", "bug = \"\"\n", `labels.bug "": want #RGB or #RRGGBB`},
		{"keys differing only in case", "bug = \"#F87171\"\nBug = \"#4ADE80\"\n", "labels.bug: same label as"},
	}
	for _, scope := range []string{"global", "project"} {
		for _, table := range []string{"theme", "theme.light", "theme.dark"} {
			for _, tt := range tests {
				t.Run(scope+"/"+table+"/"+tt.name, func(t *testing.T) {
					tmp := t.TempDir()
					t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "cfg"))
					t.Setenv("HOME", tmp)
					old, _ := os.Getwd()
					defer os.Chdir(old)
					if err := os.Chdir(tmp); err != nil {
						t.Fatal(err)
					}
					theme := "\n[" + table + ".labels]\n" + tt.labels
					if scope == "global" {
						write(t, filepath.Join(tmp, "cfg", "lanes", "config.toml"), base+theme)
					} else {
						write(t, filepath.Join(tmp, "cfg", "lanes", "config.toml"), base)
						write(t, filepath.Join(tmp, ".lanes.toml"), theme)
					}
					_, err := Load()
					if tt.wantErr == "" {
						if err != nil {
							t.Fatalf("err = %v, want none", err)
						}
						return
					}
					if err == nil {
						t.Fatal("err = nil, want a config error")
					}
					if want := table + "." + tt.wantErr; !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not contain %q", err, want)
					}
				})
			}
		}
	}
}

func TestThemeLabelsProjectMerge(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "cfg"))
	t.Setenv("HOME", tmp)
	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(tmp, "cfg", "lanes", "config.toml"), `
[provider.gl]
type = "gitlab"
[[view]]
name = "a"
provider = "gl"
[theme.labels]
bug = "#111111"
docs = "#222222"
[theme.dark.labels]
bug = "#333333"
`)
	write(t, filepath.Join(tmp, ".lanes.toml"), `
[theme.labels]
bug = "#aaaaaa"
needs-info = "#bbbbbb"
[theme.light.labels]
docs = "#cccccc"
`)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		table     string
		got, want map[string]string
	}{
		{"theme.labels", c.Theme.Labels, map[string]string{"bug": "#aaaaaa", "docs": "#222222", "needs-info": "#bbbbbb"}},
		{"theme.light.labels", c.Theme.Light.Labels, map[string]string{"docs": "#cccccc"}},
		{"theme.dark.labels", c.Theme.Dark.Labels, map[string]string{"bug": "#333333"}},
	}
	for _, tt := range tests {
		if !reflect.DeepEqual(tt.got, tt.want) {
			t.Errorf("[%s] = %v, want %v (merged per key)", tt.table, tt.got, tt.want)
		}
	}
}

func TestThemeLabelsProjectOverridesCaseVariant(t *testing.T) {
	const global = `
[provider.gl]
type = "gitlab"
[[view]]
name = "a"
provider = "gl"
[theme.labels]
bug = "#111111"
docs = "#222222"
[theme.dark.labels]
bug = "#333333"
[theme.light.labels]
bug = "#444444"
`
	tests := []struct {
		name    string
		project string
		want    map[string]string // theme.labels after loading
		wantDk  map[string]string // theme.dark.labels after loading
		wantErr string
	}{
		{"project key replaces global key differing in case", "[theme.labels]\nBug = \"#aaaaaa\"\n",
			map[string]string{"Bug": "#aaaaaa", "docs": "#222222"}, map[string]string{"bug": "#333333"}, ""},
		{"same for a mode table", "[theme.dark.labels]\nBUG = \"#aaaaaa\"\n",
			map[string]string{"bug": "#111111", "docs": "#222222"}, map[string]string{"BUG": "#aaaaaa"}, ""},
		{"light table", "[theme.light.labels]\nBug = \"#aaaaaa\"\n",
			map[string]string{"bug": "#111111", "docs": "#222222"}, map[string]string{"bug": "#333333"}, ""},
		{"exact match still overrides", "[theme.labels]\nbug = \"#aaaaaa\"\n",
			map[string]string{"bug": "#aaaaaa", "docs": "#222222"}, map[string]string{"bug": "#333333"}, ""},
		{"two keys in the project table still conflict", "[theme.labels]\nBug = \"#aaaaaa\"\nBUG = \"#bbbbbb\"\n",
			nil, nil, "labels.Bug: same label as theme.labels.BUG"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "cfg"))
			t.Setenv("HOME", tmp)
			old, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chdir(old) })
			if err := os.Chdir(tmp); err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(tmp, "cfg", "lanes", "config.toml"), global)
			write(t, filepath.Join(tmp, ".lanes.toml"), tt.project)
			c, err := Load()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(c.Theme.Labels, tt.want) {
				t.Errorf("theme.labels = %v, want %v", c.Theme.Labels, tt.want)
			}
			if !reflect.DeepEqual(c.Theme.Dark.Labels, tt.wantDk) {
				t.Errorf("theme.dark.labels = %v, want %v", c.Theme.Dark.Labels, tt.wantDk)
			}
		})
	}
}

func TestActionWhenFilterSurvivesProjectOverlay(t *testing.T) {
	tmp := t.TempDir()
	proj := filepath.Join(tmp, "proj")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "cfg"))
	t.Setenv("HOME", tmp)
	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(proj); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(tmp, "cfg", "lanes", "config.toml"), `
[provider.gl]
type = "gitlab"
[[view]]
name = "a"
provider = "gl"
[action.shared]
run = "echo shared"
when = 'kind == "ticket"'
[action.over]
run = "echo global"
when = 'kind == "ticket"'
`)
	write(t, filepath.Join(proj, ProjectFile), `
[action.over]
run = "echo project"
when = 'kind == "pr"'
`)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		action string
		kind   string
		want   bool
	}{
		{"shared", "ticket", true},
		{"shared", "pr", false},
		{"over", "pr", true},
		{"over", "ticket", false},
	}
	for _, tt := range tests {
		t.Run(tt.action+"/"+tt.kind, func(t *testing.T) {
			f := c.Action[tt.action].WhenFilter
			if f == nil {
				t.Fatal("WhenFilter is nil")
			}
			got, err := f.Match(map[string]any{"kind": tt.kind})
			if err != nil || got != tt.want {
				t.Errorf("Match(kind=%s) = %v, %v; want %v", tt.kind, got, err, tt.want)
			}
		})
	}
}
