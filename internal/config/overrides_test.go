package config

import (
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
