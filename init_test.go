package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/chriha/lanes/internal/config"
)

func TestRunInit(t *testing.T) {
	github := func() (string, string, error) { return "github", "o/r", nil }
	noOrigin := func() (string, string, error) { return "", "", errors.New("no origin") }
	tests := []struct {
		name       string
		existing   string // content of an existing .lanes.toml; "" = none
		detect     func() (string, string, error)
		wantCode   int
		wantStderr string
		wantFile   func(dir string) string
	}{
		{"writes the template", "", github, 0, "", func(string) string { return config.ProjectTemplate("github", "o/r") }},
		{"no origin", "", noOrigin, 0, "", func(string) string { return config.ProjectTemplate("", "") }},
		{"refuses an existing file", "# mine\n", github, 1, "lanes: .lanes.toml already exists\n",
			func(string) string { return "# mine\n" }},
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
			var stdout, stderr bytes.Buffer
			if code := runInit(&stdout, &stderr, dir, tt.detect); code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if stderr.String() != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.wantStderr)
			}
			if tt.wantCode == 0 && stdout.String() != path+"\n" {
				t.Errorf("stdout = %q, want the path %q", stdout.String(), path)
			}
			if b, _ := os.ReadFile(path); string(b) != tt.wantFile(dir) {
				t.Errorf("file =\n%s\nwant\n%s", b, tt.wantFile(dir))
			}
		})
	}
}
