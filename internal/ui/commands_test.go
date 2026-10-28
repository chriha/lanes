package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/item"
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

func TestFilterSave(t *testing.T) {
	const global = `# my views
[provider.p]
type = "command"
command = "true"

[[view]]
name = "v"   # keep me
provider = "p"
filter = 'true'
`
	tests := []struct {
		name     string
		view     string // view the cursor is on
		ov       config.Overrides
		notice   string
		wantFile string // "" = unchanged
	}{
		{
			name: "config view with an edit", view: "v",
			ov:       config.Overrides{Filters: map[string]string{"v": "author == me"}},
			wantFile: strings.Replace(global, "filter = 'true'", "filter = 'author == me'", 1),
		},
		{
			name: "newview view refuses", view: "n", notice: "only config views can be saved",
			ov: config.Overrides{Filters: map[string]string{}, Views: []config.View{{Name: "n", Provider: "p", Filter: "draft"}}},
		},
		{
			name: "no edit", view: "v", notice: "filter: nothing to save",
			ov: config.Overrides{Filters: map[string]string{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("HOME", tmp)
			for _, k := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
				t.Setenv(k, filepath.Join(tmp, k))
			}
			old, _ := os.Getwd()
			defer os.Chdir(old)
			if err := os.Chdir(tmp); err != nil { // no .lanes.toml: the global file defines v
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(config.Path()), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(config.Path(), []byte(global), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load()
			if err != nil {
				t.Fatal(err)
			}
			ov := tt.ov
			m := &Model{
				ov: &ov, data: map[string][]item.Item{}, fetchedAt: map[string]time.Time{}, loading: map[string]bool{},
				provErr: map[string]error{}, fails: map[string]int{}, retryAt: map[string]time.Time{}, marks: map[string]bool{},
			}
			m.applyConfig(cfg)
			if err := m.ov.Save(); err != nil {
				t.Fatal(err)
			}
			for i, v := range m.views {
				if v.cfg.Name == tt.view {
					m.switchView(i)
				}
			}

			m.runCommand("filter save")

			b, err := os.ReadFile(config.Path())
			if err != nil {
				t.Fatal(err)
			}
			want := tt.wantFile
			if want == "" {
				want = global
			}
			if string(b) != want {
				t.Errorf("config =\n%s\nwant\n%s", b, want)
			}
			if tt.notice != "" {
				if m.notice != tt.notice {
					t.Errorf("notice = %q, want %q", m.notice, tt.notice)
				}
				return
			}
			if want := "filter saved to " + config.Path(); m.notice != want {
				t.Errorf("notice = %q, want %q", m.notice, want)
			}
			saved, err := config.LoadOverrides()
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := saved.Filters["v"]; ok {
				t.Errorf("views.json still has the override: %v", saved.Filters)
			}
			if v := m.views[m.cur]; v.edited || v.cfg.Filter != "author == me" {
				t.Errorf("view: edited=%v filter=%q, want not edited, filter %q", v.edited, v.cfg.Filter, "author == me")
			}
		})
	}
}
