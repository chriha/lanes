package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOverrides(t *testing.T) {
	tests := []struct {
		name    string
		body    *string // nil = no file
		wantErr bool
		want    string // filter override for "review"
	}{
		{name: "missing", body: nil},
		{name: "valid", body: ptr(`{"filters":{"review":"draft"}}`), want: "draft"},
		{name: "corrupt", body: ptr(`{"filters":`), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			if tt.body != nil {
				write(t, overridesPath(), *tt.body)
			}
			o, err := LoadOverrides()
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if o == nil || o.Filters == nil {
				t.Fatalf("overrides must be usable, got %+v", o)
			}
			if o.Filters["review"] != tt.want {
				t.Errorf("filter = %q, want %q", o.Filters["review"], tt.want)
			}
			if tt.wantErr {
				if got := filepath.Base(overridesPath()); !strings.Contains(err.Error(), got) {
					t.Errorf("error %q should name %s", err, got)
				}
			}
		})
	}
}

func ptr(s string) *string { return &s }

func TestConfigFilesArePrivate(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	write(t, overridesPath(), `{}`)
	if err := os.Chmod(overridesPath(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (&Overrides{}).Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(filepath.Dir(Path())); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("config dir = %v, %v; want 0700", fi, err)
	}
	for _, p := range []string{overridesPath(), Path()} {
		if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("%s = %v, %v; want 0600", p, fi, err)
		}
	}
}

func TestSaveKeepsCorruptOverrides(t *testing.T) {
	tests := []struct {
		name       string
		body       *string // nil = no file
		wantBackup bool
	}{
		{name: "missing", body: nil},
		{name: "valid", body: ptr(`{"filters":{"a":"b"}}`)},
		{name: "corrupt", body: ptr(`{"filters":`), wantBackup: true},
		{name: "empty", body: ptr(``), wantBackup: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			if tt.body != nil {
				write(t, overridesPath(), *tt.body)
			}
			o := &Overrides{Filters: map[string]string{"review": "draft"}}
			if err := o.Save(); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(overridesPath() + ".bak")
			if got := err == nil; got != tt.wantBackup {
				t.Fatalf("backup exists = %v, want %v", got, tt.wantBackup)
			}
			if tt.wantBackup {
				if string(b) != *tt.body {
					t.Errorf("backup = %q, want %q", b, *tt.body)
				}
				if o.BackedUp() != overridesPath()+".bak" {
					t.Errorf("BackedUp() = %q", o.BackedUp())
				}
			} else if o.BackedUp() != "" {
				t.Errorf("BackedUp() = %q, want empty", o.BackedUp())
			}
			saved, err := LoadOverrides()
			if err != nil || saved.Filters["review"] != "draft" {
				t.Errorf("saved = %+v, %v", saved, err)
			}
		})
	}
}
