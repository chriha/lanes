package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chriha/lanes/internal/config"
)

func TestEditProjectCreatesTemplate(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "cfg"))
	proj := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.Chdir(proj); err != nil {
		t.Fatal(err)
	}
	m := &Model{originForge: func() (string, string, error) { return "gitlab", "acme/payments", nil }}
	if _, cmd := m.runCommand("edit project"); cmd == nil {
		t.Fatal("no editor command returned")
	}
	b, err := os.ReadFile(filepath.Join(proj, config.ProjectFile))
	if err != nil {
		t.Fatal(err)
	}
	if want := config.ProjectTemplate("gitlab", "acme/payments"); string(b) != want {
		t.Errorf(".lanes.toml =\n%s\nwant the project template\n%s", b, want)
	}
}
