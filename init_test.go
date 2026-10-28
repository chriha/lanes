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

func TestRunInit(t *testing.T) {
	github := func() (string, string, error) { return "github", "o/r", nil }
	noOrigin := func() (string, string, error) { return "", "", errors.New("no origin") }
	mine := func(string) string { return "# mine\n" }
	tmpl := func(string) string { return config.ProjectTemplate("github", "o/r") }
	tests := []struct {
		name       string
		existing   string // content of an existing .lanes.toml; "" = none
		oldBak     string // content of an existing .lanes.toml.bak; "" = none
		input      string // what the user types
		tty        bool
		detect     func() (string, string, error)
		wantCode   int
		wantStdout bool   // the path is printed
		wantBak    string // expected .lanes.toml.bak content; "" = none
		wantStderr string
		wantFile   func(dir string) string
	}{
		{"writes the template", "", "", "", false, github, 0, true, "", "", func(string) string { return config.ProjectTemplate("github", "o/r") }},
		{"no origin", "", "", "", false, noOrigin, 0, true, "", "", func(string) string { return config.ProjectTemplate("", "") }},
		{"no tty refuses", "# mine\n", "", "y\n\n", false, github, 1, false, "", "lanes: .lanes.toml already exists\n", mine},
		{"yes, backup by default", "# mine\n", "", "y\n\n", true, github, 0, true, "# mine\n", "backed up to %BAK%\n", tmpl},
		{"YES, backup y", "# mine\n", "", "YES\ny\n", true, github, 0, true, "# mine\n", "backed up to %BAK%\n", tmpl},
		{"yes, backup n keeps an old bak", "# mine\n", "# old\n", "yes\nn\n", true, github, 0, true, "# old\n", "", tmpl},
		{"yes, backup replaces an old bak", "# mine\n", "# old\n", "y\n\n", true, github, 0, true, "# mine\n", "backed up to %BAK%\n", tmpl},
		{"enter declines", "# mine\n", "", "\n", true, github, 0, false, "", "", mine},
		{"n declines", "# mine\n", "", "n\n", true, github, 0, false, "", "", mine},
		{"other text declines", "# mine\n", "", "sure\n", true, github, 0, false, "", "", mine},
		{"EOF declines", "# mine\n", "", "", true, github, 0, false, "", "", mine},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, config.ProjectFile)
			if tt.existing != "" {
				if err := os.WriteFile(path, []byte(tt.existing), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			bakPath := path + ".bak"
			if tt.oldBak != "" {
				if err := os.WriteFile(bakPath, []byte(tt.oldBak), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var stdout, stderr bytes.Buffer
			if code := runInit(strings.NewReader(tt.input), tt.tty, &stdout, &stderr, dir, tt.detect); code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			wantErr := strings.ReplaceAll(tt.wantStderr, "%BAK%", bakPath)
			if !tt.tty { // no prompts: stderr is exactly the message
				if stderr.String() != wantErr {
					t.Errorf("stderr = %q, want %q", stderr.String(), wantErr)
				}
			} else if strings.Contains(stderr.String(), "backed up to") != (wantErr != "") ||
				!strings.Contains(stderr.String(), wantErr) {
				t.Errorf("stderr = %q, want to contain %q", stderr.String(), wantErr)
			}
			if wantOut := ""; tt.wantStdout {
				wantOut = path + "\n"
				if stdout.String() != wantOut {
					t.Errorf("stdout = %q, want %q", stdout.String(), wantOut)
				}
			} else if stdout.String() != "" {
				t.Errorf("stdout = %q, want nothing", stdout.String())
			}
			bak, err := os.ReadFile(bakPath)
			if tt.wantBak == "" && err == nil || tt.wantBak != "" && string(bak) != tt.wantBak {
				t.Errorf(".bak = %q (err %v), want %q", bak, err, tt.wantBak)
			}
			if b, _ := os.ReadFile(path); string(b) != tt.wantFile(dir) {
				t.Errorf("file =\n%s\nwant\n%s", b, tt.wantFile(dir))
			}
		})
	}
}
