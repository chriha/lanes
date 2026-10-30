package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestProjectTemplateForge(t *testing.T) {
	tests := []struct {
		name      string
		typ, repo string
		want      *Provider // nil: no active provider
	}{
		{"github", "github", "o/r", &Provider{Type: "github", Project: "o/r"}},
		{"gitlab.com", "gitlab", "group/sub/project", &Provider{Type: "gitlab", Project: "group/sub/project"}},
		{"self-hosted gitlab", "gitlab", "acme/payments", &Provider{Type: "gitlab", Project: "acme/payments"}},
		{"no origin", "", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := ProjectTemplate(tt.typ, tt.repo)
			var c Config
			if _, err := toml.Decode(tmpl, &c); err != nil {
				t.Fatalf("template does not parse: %v\n%s", err, tmpl)
			}
			if tt.want == nil {
				if len(c.Provider) != 0 {
					t.Errorf("providers = %v, want none active", c.Provider)
				}
				if !strings.Contains(tmpl, "origin could not be detected") {
					t.Errorf("template does not say the origin could not be detected:\n%s", tmpl)
				}
				return
			}
			if len(c.Provider) != 1 || !reflect.DeepEqual(c.Provider["forge"], *tt.want) {
				t.Errorf("providers = %+v, want only forge = %+v", c.Provider, *tt.want)
			}
		})
	}
}

// The generated file must not change anything but the forge provider when overlaid on the global config.
func TestProjectTemplateOverlayKeepsGlobal(t *testing.T) {
	tests := []struct {
		name      string
		typ, repo string
		forge     Provider
	}{
		{"github", "github", "o/r", Provider{Type: "github", Project: "o/r"}},
		{"gitlab", "gitlab", "g/p", Provider{Type: "gitlab", Project: "g/p"}},
		{"no origin", "", "", Provider{Type: "auto"}},
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
			global, err := Load() // writes the default global config
			if err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(proj, ProjectFile), ProjectTemplate(tt.typ, tt.repo))
			got, err := Load()
			if err != nil {
				t.Fatalf("load with project config: %v", err)
			}
			if !reflect.DeepEqual(got.View, global.View) || !reflect.DeepEqual(got.Action, global.Action) ||
				!reflect.DeepEqual(got.Keymap, global.Keymap) || got.Settings != global.Settings || !reflect.DeepEqual(got.Theme, global.Theme) {
				t.Errorf("project template changed more than the forge provider:\ngot  %+v\nwant %+v", got, global)
			}
			want := map[string]Provider{"forge": tt.forge}
			if !reflect.DeepEqual(got.Provider, want) {
				t.Errorf("providers = %+v, want %+v", got.Provider, want)
			}
		})
	}
}

// Every field of the schema is documented, so adding one without updating the template fails here.
func TestProjectTemplateDocumentsEveryField(t *testing.T) {
	tmpl := ProjectTemplate("", "")
	for _, s := range []any{Settings{}, Provider{}, View{}, Action{}, Theme{}, Colors{}} {
		typ := reflect.TypeOf(s)
		for i := 0; i < typ.NumField(); i++ {
			tag, _, _ := strings.Cut(typ.Field(i).Tag.Get("toml"), ",")
			if tag == "" || tag == "-" || typ.Field(i).Type.Kind() == reflect.Struct { // subtables: checked as sections below
				continue
			}
			if !regexp.MustCompile(`(?m)^#? ?` + regexp.QuoteMeta(tag) + ` +=`).MatchString(tmpl) {
				t.Errorf("%s.%s (toml %q) is not in the project template", typ.Name(), typ.Field(i).Name, tag)
			}
		}
	}
	for _, section := range []string{"[settings]", "[provider.", "[[view]]", "[keymap]", "[action.", "[theme]", "[theme.light]", "[theme.dark]"} {
		if !strings.Contains(tmpl, section) {
			t.Errorf("section %s is not in the project template", section)
		}
	}
}

func TestInitProject(t *testing.T) {
	t.Run("creates the file", func(t *testing.T) {
		dir := t.TempDir()
		path, err := InitProject(dir, "github", "o/r")
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(dir, ProjectFile); path != want {
			t.Errorf("path = %q, want %q", path, want)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != ProjectTemplate("github", "o/r") {
			t.Errorf("file content is not the template:\n%s", b)
		}
	})
	t.Run("refuses an existing file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ProjectFile)
		write(t, path, "# mine\n")
		if _, err := InitProject(dir, "github", "o/r"); !errors.Is(err, fs.ErrExist) {
			t.Errorf("err = %v, want fs.ErrExist", err)
		}
		if b, _ := os.ReadFile(path); string(b) != "# mine\n" {
			t.Errorf("existing file changed to %q", b)
		}
	})
}

func TestOverwriteProject(t *testing.T) {
	tests := []struct {
		name     string
		existing bool
		oldBak   string
		backup   bool
		wantBak  string // "" = no .bak
	}{
		{"no existing file", false, "", true, ""},
		{"backup", true, "", true, "# mine\n"},
		{"backup replaces an older one", true, "# old\n", true, "# mine\n"},
		{"no backup leaves an older one", true, "# old\n", false, "# old\n"},
		{"no backup, none before", true, "", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ProjectFile)
			if tt.existing {
				write(t, path, "# mine\n")
			}
			if tt.oldBak != "" {
				write(t, path+".bak", tt.oldBak)
			}
			got, err := OverwriteProject(dir, "github", "o/r", tt.backup)
			if err != nil || got != path {
				t.Fatalf("OverwriteProject = %q, %v; want %q", got, err, path)
			}
			if b, _ := os.ReadFile(path); string(b) != ProjectTemplate("github", "o/r") {
				t.Errorf("file = %q, want the template", b)
			}
			bak, err := os.ReadFile(path + ".bak")
			if tt.wantBak == "" && err == nil || tt.wantBak != "" && string(bak) != tt.wantBak {
				t.Errorf(".bak = %q (err %v), want %q", bak, err, tt.wantBak)
			}
		})
	}
}

// InitProject keeps refusing to overwrite (`:edit project` relies on it).
func TestInitProjectNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ProjectFile)
	write(t, path, "# mine\n")
	if _, err := InitProject(dir, "github", "o/r"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("err = %v, want ErrExist", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "# mine\n" {
		t.Errorf("file = %q, want unchanged", b)
	}
}
