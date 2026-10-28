package config

import (
	"os"
	"path/filepath"
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
accent = "#111111"
ok = "#222222"
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
ok = "#333333"
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
	if c.Theme.Accent != "#111111" || c.Theme.OK != "#333333" {
		t.Errorf("theme should merge per field: %+v", c.Theme)
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
