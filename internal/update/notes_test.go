package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestClean(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"goreleaser", "## Changelog\n### Features\n* a1b2c3d4e5f6: add thing (@chriha)\n### Fixes\n- 0123456: fix it (@bot-user)\n", "### Features\n* add thing\n### Fixes\n- fix it"},
		{"empty", "", ""},
		{"only heading", "## Changelog\n", ""},
		{"crlf", "## Changelog\r\n* abcdef0: x (@a)\r\n", "* x"},
		{"odd body kept", "Just some prose\n* not a sha: keep (really)", "Just some prose\n* not a sha: keep (really)"},
		{"mention mid-line kept", "* abcdef0: thanks (@a) for this", "* thanks (@a) for this"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Clean(tt.in); got != tt.want {
				t.Errorf("Clean() = %q, want %q", got, tt.want)
			}
		})
	}
}

func tags(rs []Release) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Tag)
	}
	return out
}

func TestBetween(t *testing.T) {
	rels := []Release{{Tag: "v1.0.0"}, {Tag: "v1.3.0"}, {Tag: "junk"}, {Tag: "v1.2.0"}, {Tag: "v1.1.0"}, {Tag: "v1.4.0"}}
	tests := []struct {
		name          string
		last, current string
		max           int
		want          []string
	}{
		{"range newest first", "v1.0.0", "v1.3.0", 10, []string{"v1.3.0", "v1.2.0", "v1.1.0"}},
		{"cap", "v1.0.0", "v1.4.0", 2, []string{"v1.4.0", "v1.3.0"}},
		{"same version", "v1.3.0", "v1.3.0", 10, nil},
		{"downgrade", "v1.4.0", "v1.2.0", 10, nil},
		{"unknown last", "dev", "v1.3.0", 10, nil},
		{"dev current", "v1.0.0", "dev", 10, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tags(Between(rels, tt.last, tt.current, tt.max)); !slices.Equal(got, tt.want) {
				t.Errorf("Between() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLastRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "last_version")
	if _, ok := LastRun(path); ok {
		t.Fatal("missing file reported a version")
	}
	if err := SaveLastRun(path, "v1.2.0"); err != nil {
		t.Fatal(err)
	}
	if v, ok := LastRun(path); !ok || v != "v1.2.0" {
		t.Errorf("LastRun() = %q, %v; want v1.2.0, true", v, ok)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, %v; want 0600", fi.Mode().Perm(), err)
	}
	if err := os.WriteFile(path, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := LastRun(path); ok {
		t.Error("corrupt file reported a version")
	}
}

func TestFetchNotes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		want    []string
		wantErr bool
	}{
		{"skips drafts and prereleases", 200, `[{"tag_name":"v1.2.0","body":"b"},{"tag_name":"v1.3.0-rc.1","prerelease":true},{"tag_name":"v1.4.0","draft":true},{"tag_name":"v1.1.0"}]`, []string{"v1.2.0", "v1.1.0"}, false},
		{"http error", 500, ``, nil, true},
		{"bad json", 200, `{`, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			got, err := fetchNotes(context.Background(), srv.Client(), srv.URL)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !slices.Equal(tags(got), tt.want) {
				t.Errorf("tags = %v, want %v", tags(got), tt.want)
			}
		})
	}
}
