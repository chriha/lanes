package update

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"v1.0.1", "v1.0.0", true},
		{"v1.1.0", "v1.0.9", true},
		{"v2.0.0", "v1.99.99", true},
		{"v1.10.0", "v1.9.0", true}, // numeric, not lexical
		{"v1.0.0", "v1.0.0", false},
		{"v1.0.0", "v1.0.1", false},
		{"v1.2.0", "v1.2.0-rc1", true}, // release beats its prerelease
		{"v1.2.0-rc1", "v1.2.0", false},
		{"v1.2.0-rc.2", "v1.2.0-rc.1", true},
		{"v1.2.0-rc.10", "v1.2.0-rc.9", true}, // numeric prerelease identifiers
		{"v1.2.0-rc.1", "v1.2.0-rc", true},    // more identifiers sort higher
		{"v1.2.0-beta", "v1.2.0-alpha", true},
		{"v1.2.0-alpha", "v1.2.0-1", true}, // alphanumeric beats numeric
		{"v1.2.0-rc1", "v1.1.9", true},
		{"v1.0.1+build.5", "v1.0.0", true}, // build metadata is ignored
		{"v1.0.0+build.5", "v1.0.0", false},
		// malformed on either side never counts as newer
		{"v1.0.1", "dev", false},
		{"dev", "v1.0.0", false},
		{"1.0.1", "v1.0.0", false},
		{"v1.0", "v0.9.0", false},
		{"v1.0.0.1", "v0.9.0", false},
		{"v1.x.0", "v0.9.0", false},
		{"v-1.0.0", "v0.9.0", false},
		{"v1.0.0-", "v0.9.0", false},
		{"v1.0.0-rc..1", "v0.9.0", false},
		{"v01.0.0", "v0.9.0", false}, // leading zeros
		{"v1.00.0", "v0.9.0", false},
		{"v1.0.0-01", "v0.9.0", false},
		{"v1.0.0-rc_1", "v0.9.0", false}, // outside [0-9A-Za-z-]
		{"v1.0.0-rcé", "v0.9.0", false},
		{"v1.0.0-0", "v0.9.0", true}, // zero itself is fine
		{"v1.0.0-x-y.0a", "v0.9.0", true},
		{"", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			if got := Newer(tt.a, tt.b); got != tt.want {
				t.Errorf("Newer(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

var t0 = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

// fetcher counts calls and answers with tag or err.
type fetcher struct {
	tag   string
	err   error
	calls int
}

func (f *fetcher) fetch(context.Context) (string, error) {
	f.calls++
	return f.tag, f.err
}

func writeState(t *testing.T, path string, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLatestThrottle(t *testing.T) {
	fresh := `{"checked_at":"` + t0.Add(-23*time.Hour).Format(time.RFC3339) + `","latest":"v1.1.0"}`
	stale := `{"checked_at":"` + t0.Add(-25*time.Hour).Format(time.RFC3339) + `","latest":"v1.1.0"}`
	future := `{"checked_at":"` + t0.Add(48*time.Hour).Format(time.RFC3339) + `","latest":"v1.1.0"}`
	tests := []struct {
		name       string
		current    string
		cache      string // "" = no cache file
		fetch      fetcher
		wantCalls  int
		wantLatest string
		wantNewer  bool
		wantErr    bool
		wantCache  string // latest in the cache file afterwards
		wantAt     time.Time
	}{
		{"no cache fetches", "v1.0.0", "", fetcher{tag: "v1.2.0"}, 1, "v1.2.0", true, false, "v1.2.0", t0},
		{"fresh cache skips network", "v1.0.0", fresh, fetcher{tag: "v1.2.0"}, 0, "v1.1.0", true, false, "v1.1.0", t0.Add(-23 * time.Hour)},
		{"stale cache fetches", "v1.0.0", stale, fetcher{tag: "v1.2.0"}, 1, "v1.2.0", true, false, "v1.2.0", t0},
		{"checked_at in the future counts as stale", "v1.0.0", future, fetcher{tag: "v1.2.0"}, 1, "v1.2.0", true, false, "v1.2.0", t0},
		{"corrupt cache fetches", "v1.0.0", "{not json", fetcher{tag: "v1.2.0"}, 1, "v1.2.0", true, false, "v1.2.0", t0},
		{"same version is not newer", "v1.2.0", "", fetcher{tag: "v1.2.0"}, 1, "v1.2.0", false, false, "v1.2.0", t0},
		{"older release is not newer", "v1.3.0", "", fetcher{tag: "v1.2.0"}, 1, "v1.2.0", false, false, "v1.2.0", t0},
		{"failed fetch keeps checked_at", "v1.0.0", stale, fetcher{err: errors.New("offline")}, 1, "", false, true, "v1.1.0", t0.Add(-25 * time.Hour)},
		{"malformed tag is a failure", "v1.0.0", "", fetcher{tag: "latest"}, 1, "", false, true, "", time.Time{}},
		{"dev build never checks", "dev", "", fetcher{tag: "v1.2.0"}, 0, "", false, false, "", time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "update.json")
			if tt.cache != "" {
				writeState(t, path, tt.cache)
			}
			f := tt.fetch
			c := Checker{Current: tt.current, Fetch: f.fetch, Now: func() time.Time { return t0 }, CachePath: path}
			latest, newer, err := c.Latest(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if latest != tt.wantLatest || newer != tt.wantNewer {
				t.Errorf("Latest = (%q, %v), want (%q, %v)", latest, newer, tt.wantLatest, tt.wantNewer)
			}
			if f.calls != tt.wantCalls {
				t.Errorf("fetch calls = %d, want %d", f.calls, tt.wantCalls)
			}
			s, ok := load(path)
			if tt.wantCache == "" {
				if ok && tt.cache == "" {
					t.Errorf("cache written = %+v, want none", s)
				}
				return
			}
			if !ok || s.Latest != tt.wantCache || !s.CheckedAt.Equal(tt.wantAt) {
				t.Errorf("cache = %+v (ok %v), want latest %q at %v", s, ok, tt.wantCache, tt.wantAt)
			}
		})
	}
}

// A day of starts makes one request; the next day makes the second.
func TestLatestOncePerDay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.json")
	f := &fetcher{tag: "v1.2.0"}
	now := t0
	c := Checker{Current: "v1.0.0", Fetch: f.fetch, Now: func() time.Time { return now }, CachePath: path}
	for _, step := range []time.Duration{0, time.Hour, 12 * time.Hour, 23*time.Hour + 59*time.Minute} {
		now = t0.Add(step)
		if _, _, err := c.Latest(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if f.calls != 1 {
		t.Fatalf("calls within 24h = %d, want 1", f.calls)
	}
	now = t0.Add(24 * time.Hour)
	if _, _, err := c.Latest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.calls != 2 {
		t.Errorf("calls after 24h = %d, want 2", f.calls)
	}
}

// A failed fetch is retried on the next start, not a day later.
func TestLatestRetriesAfterFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.json")
	f := &fetcher{err: errors.New("offline")}
	c := Checker{Current: "v1.0.0", Fetch: f.fetch, Now: func() time.Time { return t0 }, CachePath: path}
	if _, newer, err := c.Latest(context.Background()); err == nil || newer {
		t.Fatalf("failed fetch = (newer %v, err %v), want an error and no newer", newer, err)
	}
	f.tag, f.err = "v1.2.0", nil
	latest, newer, err := c.Latest(context.Background())
	if err != nil || latest != "v1.2.0" || !newer {
		t.Errorf("retry = (%q, %v, %v), want (v1.2.0, true, nil)", latest, newer, err)
	}
	if f.calls != 2 {
		t.Errorf("calls = %d, want 2", f.calls)
	}
}

func TestCacheFileMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache") // missing dir is created
	path := filepath.Join(dir, "update.json")
	f := &fetcher{tag: "v1.2.0"}
	c := Checker{Current: "v1.0.0", Fetch: f.fetch, Now: func() time.Time { return t0 }, CachePath: path}
	if _, _, err := c.Latest(context.Background()); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
	}
}

func TestCached(t *testing.T) {
	tests := []struct {
		name       string
		current    string
		cache      string
		wantLatest string
		wantNewer  bool
	}{
		{"missing", "v1.0.0", "", "", false},
		{"corrupt", "v1.0.0", "{", "", false},
		{"newer", "v1.0.0", `{"checked_at":"2020-01-01T00:00:00Z","latest":"v1.1.0"}`, "v1.1.0", true},
		{"same", "v1.1.0", `{"checked_at":"2020-01-01T00:00:00Z","latest":"v1.1.0"}`, "v1.1.0", false},
		{"older", "v1.2.0", `{"checked_at":"2020-01-01T00:00:00Z","latest":"v1.1.0"}`, "v1.1.0", false},
		{"dev build", "dev", `{"checked_at":"2020-01-01T00:00:00Z","latest":"v1.1.0"}`, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "update.json")
			if tt.cache != "" {
				writeState(t, path, tt.cache)
			}
			f := &fetcher{err: errors.New("Cached must not fetch")}
			c := Checker{Current: tt.current, Fetch: f.fetch, Now: func() time.Time { return t0 }, CachePath: path}
			latest, newer := c.Cached()
			if latest != tt.wantLatest || newer != tt.wantNewer {
				t.Errorf("Cached = (%q, %v), want (%q, %v)", latest, newer, tt.wantLatest, tt.wantNewer)
			}
			if f.calls != 0 {
				t.Errorf("fetch calls = %d, want 0", f.calls)
			}
		})
	}
}

func TestGitHubFetch(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		want    string
		wantErr bool
	}{
		{"ok", http.StatusOK, `{"tag_name":"v1.2.0","draft":false}`, "v1.2.0", false},
		{"not found", http.StatusNotFound, `{"message":"Not Found"}`, "", true},
		{"rate limited", http.StatusForbidden, `{"message":"API rate limit exceeded"}`, "", true},
		{"bad json", http.StatusOK, `{`, "", true},
		{"no tag", http.StatusOK, `{}`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Accept") != "application/vnd.github+json" {
					t.Errorf("Accept = %q", r.Header.Get("Accept"))
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			got, err := fetchFrom(context.Background(), srv.Client(), srv.URL)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("fetch = (%q, %v), want (%q, err %v)", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestHint(t *testing.T) {
	goInstall := &debug.BuildInfo{Main: debug.Module{Path: "github.com/chriha/lanes", Version: "v1.2.0"}}
	localBuild := &debug.BuildInfo{
		Main:     debug.Module{Path: "github.com/chriha/lanes", Version: "v1.2.1-0.20261101120000-abcdef123456"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abcdef123456"}},
	}
	devel := &debug.BuildInfo{Main: debug.Module{Path: "github.com/chriha/lanes", Version: "(devel)"}}
	tests := []struct {
		name string
		exe  string
		bi   *debug.BuildInfo
		want string
	}{
		{"homebrew cellar intel", "/usr/local/Cellar/lanes/1.2.0/bin/lanes", goInstall, Brew},
		{"homebrew apple silicon", "/opt/homebrew/Cellar/lanes/1.2.0/bin/lanes", nil, Brew},
		{"homebrew opt symlink target", "/opt/homebrew/bin/lanes", nil, Brew},
		{"linuxbrew", "/home/linuxbrew/.linuxbrew/Cellar/lanes/1.2.0/bin/lanes", nil, Brew},
		{"go install", "/Users/me/go/bin/lanes", goInstall, GoInstall},
		{"local go build stamps vcs", "/Users/me/lanes/bin/lanes", localBuild, Releases},
		{"devel build", "/Users/me/lanes/bin/lanes", devel, Releases},
		{"no build info", "/usr/local/bin/lanes", nil, Releases},
		{"empty exe", "", nil, Releases},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Hint(tt.exe, tt.bi); got != tt.want {
				t.Errorf("Hint(%q) = %q, want %q", tt.exe, got, tt.want)
			}
		})
	}
}
