package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
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
	if c.Settings.PollInterval != "45s" || c.Settings.Preview || c.Settings.ClaudeDir != "~/.claude-personal" {
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
	if want := (Colors{Accent: "#aaaaaa", OK: "#dddddd"}); c.Theme.Light != want {
		t.Errorf("[theme.light] should merge per field: %+v, want %+v", c.Theme.Light, want)
	}
	if want := (Colors{Info: "#cccccc", Warn: "#eeeeee"}); c.Theme.Dark != want {
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
