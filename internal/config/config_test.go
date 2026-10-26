package config

import (
	"os"
	"path/filepath"
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
	if c.Settings.PollInterval != "45s" || c.Settings.Preview {
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
