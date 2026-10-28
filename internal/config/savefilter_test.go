package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
)

const saveFixture = `# global config
[settings]
poll_interval = "30s"

[[view]]
name = "review"   # the main one
provider = "forge"
filter = '!draft'  # old
sort = "title"

# drafts below
[[view]]
provider = "forge"
name = 'drafts'

[[view]]
name = "multi"
provider = "forge"
filter = '''
draft
'''
`

func TestSaveViewFilter(t *testing.T) {
	tests := []struct {
		name, in, view, expr, want string
		err                        error
	}{
		{
			name: "replaces only the filter line", in: saveFixture, view: "review", expr: `author == me`,
			want: `# global config
[settings]
poll_interval = "30s"

[[view]]
name = "review"   # the main one
provider = "forge"
filter = 'author == me'
sort = "title"

# drafts below
[[view]]
provider = "forge"
name = 'drafts'

[[view]]
name = "multi"
provider = "forge"
filter = '''
draft
'''
`,
		},
		{
			name: "adds a missing filter at the end of the block", in: saveFixture, view: "drafts", expr: `draft`,
			want: `# global config
[settings]
poll_interval = "30s"

[[view]]
name = "review"   # the main one
provider = "forge"
filter = '!draft'  # old
sort = "title"

# drafts below
[[view]]
provider = "forge"
name = 'drafts'
filter = 'draft'

[[view]]
name = "multi"
provider = "forge"
filter = '''
draft
'''
`,
		},
		{
			name: "single quote becomes a basic string", in: saveFixture, view: "review", expr: `title == "it's" || branch == "a\b"`,
		},
		{name: "multi-line filter refuses", in: saveFixture, view: "multi", expr: "true", err: ErrNotSavable},
		{name: "unknown view refuses", in: saveFixture, view: "nope", expr: "true", err: ErrNotSavable},
		{name: "inline table views refuse", in: "view = [{ name = \"review\", provider = \"forge\" }]\n", view: "review", expr: "true", err: ErrNotSavable},
		{
			name: "keeps CRLF line endings", in: "[[view]]\r\nname = \"a\"\r\nfilter = 'x'\r\n", view: "a", expr: "y",
			want: "[[view]]\r\nname = \"a\"\r\nfilter = 'y'\r\n",
		},
		{
			name: "name after filter", in: "[[view]]\nfilter = 'x'\nname = \"a\"\n", view: "a", expr: "y",
			want: "[[view]]\nfilter = 'y'\nname = \"a\"\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(tt.in), 0o644); err != nil {
				t.Fatal(err)
			}
			err := SaveViewFilter(path, tt.view, tt.expr)
			b, rerr := os.ReadFile(path)
			if rerr != nil {
				t.Fatal(rerr)
			}
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Errorf("err = %v, want %v", err, tt.err)
				}
				if string(b) != tt.in {
					t.Errorf("file changed on refusal:\n%s", b)
				}
				return
			}
			if err != nil {
				t.Fatalf("SaveViewFilter: %v", err)
			}
			if tt.want != "" && string(b) != tt.want {
				t.Errorf("file =\n%s\nwant\n%s", b, tt.want)
			}
			var c Config
			if _, err := toml.Decode(string(b), &c); err != nil {
				t.Fatalf("written file does not decode: %v", err)
			}
			for _, v := range c.View {
				if v.Name == tt.view && v.Filter != tt.expr {
					t.Errorf("decoded filter = %q, want %q", v.Filter, tt.expr)
				}
			}
		})
	}
}

func TestViewFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "cfg"))
	proj := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ProjectFile), []byte("[[view]]\nname = \"review\"\nprovider = \"forge\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.Chdir(proj); err != nil {
		t.Fatal(err)
	}
	pp := filepath.Join(proj, ProjectFile)
	if real, err := filepath.EvalSymlinks(proj); err == nil { // macOS: TempDir is under a symlinked /var
		pp = filepath.Join(real, ProjectFile)
	}
	for _, tt := range []struct{ view, want string }{
		{"review", pp},   // defined in both: the project view is in effect
		{"mine", Path()}, // global only
	} {
		got, err := ViewFile(tt.view)
		if err != nil {
			t.Fatalf("ViewFile(%q): %v", tt.view, err)
		}
		if got != tt.want {
			t.Errorf("ViewFile(%q) = %q, want %q", tt.view, got, tt.want)
		}
	}
}
