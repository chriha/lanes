// Package update finds out whether a newer release than the running one exists on GitHub, at most once per
// Interval, and says how to upgrade for the way the binary was installed. It never replaces the binary itself.
package update

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/chriha/lanes/internal/app"
)

// Interval is how long a successful check is trusted before the network is asked again.
const Interval = 24 * time.Hour

// Timeout bounds one request to GitHub.
const Timeout = 5 * time.Second

// maxBody caps how much of GitHub's answer is read; a release is a few KiB.
const maxBody = 1 << 20

// repo is the GitHub repository releases come from.
var repo = "chriha/" + app.Name

// Upgrade commands per install method, as returned by Hint.
var (
	Brew      = "brew upgrade " + app.Name
	GoInstall = "go install github.com/" + repo + "@latest"
	Releases  = "https://github.com/" + repo + "/releases/latest"
)

// Checker compares Current with the newest release, throttled through the cache file at CachePath.
// Fetch and Now are injected so tests never reach the network or the real clock.
type Checker struct {
	Current   string                                    // running version, e.g. "v1.2.0"; anything not semver (e.g. "dev") never checks
	Fetch     func(ctx context.Context) (string, error) // returns the newest release tag, e.g. GitHub
	Now       func() time.Time
	CachePath string // e.g. filepath.Join(app.CacheDir(), "update.json")
}

// state is the cache file: when the last successful check happened and what it found.
type state struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
}

// Latest returns the newest release tag and whether it is newer than Current. The network is asked only when
// the cache is missing, unreadable or older than Interval; otherwise the cached tag is used. A failed fetch
// returns an error, no tag, and leaves the cache untouched so the next start tries again. When only saving the
// cache fails, the tag and newer are still valid and the error says the next start will ask again. A
// non-semver Current never fetches and returns no tag and no error.
func (c Checker) Latest(ctx context.Context) (string, bool, error) {
	if !valid(c.Current) {
		return "", false, nil
	}
	now := c.Now()
	if s, ok := load(c.CachePath); ok {
		if age := now.Sub(s.CheckedAt); age >= 0 && age < Interval {
			return s.Latest, Newer(s.Latest, c.Current), nil
		}
	}
	tag, err := c.Fetch(ctx)
	if err != nil {
		return "", false, err
	}
	if !valid(tag) {
		return "", false, fmt.Errorf("latest release %q is not a semver tag", tag)
	}
	newer := Newer(tag, c.Current)
	if err := save(c.CachePath, state{CheckedAt: now, Latest: tag}); err != nil {
		return tag, newer, fmt.Errorf("saving update check: %w", err)
	}
	return tag, newer, nil
}

// Cached returns the tag of the last successful check and whether it is newer than Current, without any
// network request. A missing or unreadable cache, or a non-semver Current, returns no tag.
func (c Checker) Cached() (string, bool) {
	if !valid(c.Current) {
		return "", false
	}
	s, ok := load(c.CachePath)
	if !ok {
		return "", false
	}
	return s.Latest, Newer(s.Latest, c.Current)
}

func load(path string) (state, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return state{}, false
	}
	var s state
	if json.Unmarshal(b, &s) != nil || !valid(s.Latest) {
		return state{}, false
	}
	return s, true
}

func save(path string, s state) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), app.DirMode); err != nil {
		return err
	}
	return app.WriteFile(path, b)
}

// GitHub returns a Fetch that asks GitHub for the latest release. That endpoint excludes drafts and
// prereleases, so unreleased tags never show up.
func GitHub() func(ctx context.Context) (string, error) {
	client := &http.Client{Timeout: Timeout}
	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	return func(ctx context.Context) (string, error) { return fetchFrom(ctx, client, url) }
}

func fetchFrom(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("latest release: %s", resp.Status)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&rel); err != nil {
		return "", fmt.Errorf("latest release: %w", err)
	}
	if rel.TagName == "" {
		return "", errors.New("latest release: no tag_name")
	}
	return rel.TagName, nil
}

// Hint returns the upgrade command for how the binary at exe (the resolved executable path) was installed:
// Homebrew by its prefix, `go install` by a module version in bi without VCS stamps (a local `go build`
// stamps both), else the releases page.
func Hint(exe string, bi *debug.BuildInfo) string {
	if strings.Contains(exe, "/Cellar/") || strings.HasPrefix(exe, "/opt/homebrew/") || strings.HasPrefix(exe, "/home/linuxbrew/") {
		return Brew
	}
	if bi != nil && bi.Main.Version != "" && bi.Main.Version != "(devel)" && !stamped(bi) {
		return GoInstall
	}
	return Releases
}

func stamped(bi *debug.BuildInfo) bool {
	for _, s := range bi.Settings {
		if s.Key == "vcs.revision" {
			return true
		}
	}
	return false
}

// version is a parsed "vMAJOR.MINOR.PATCH[-PRERELEASE][+BUILD]".
type version struct {
	core [3]int
	pre  []string // nil for a release
}

func parse(s string) (version, bool) {
	var v version
	rest, ok := strings.CutPrefix(s, "v")
	if !ok {
		return v, false
	}
	if i := strings.IndexByte(rest, '+'); i >= 0 {
		rest = rest[:i] // build metadata doesn't order versions
	}
	core, pre, hasPre := strings.Cut(rest, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, ok := number(p)
		if !ok {
			return v, false
		}
		v.core[i] = n
	}
	if hasPre {
		v.pre = strings.Split(pre, ".")
		for _, id := range v.pre {
			if !identifier(id) {
				return v, false
			}
		}
	}
	return v, true
}

func valid(s string) bool {
	_, ok := parse(s)
	return ok
}

// identifier reports whether id is a valid prerelease identifier: [0-9A-Za-z-]+, numeric ones without
// leading zeros.
func identifier(id string) bool {
	if id == "" || strings.Trim(id, "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-") != "" {
		return false
	}
	if strings.Trim(id, "0123456789") == "" {
		_, ok := number(id)
		return ok
	}
	return true
}

// number parses a non-negative decimal of digits only: no sign, no spaces, no leading zeros.
func number(s string) (int, bool) {
	if s == "" || strings.Trim(s, "0123456789") != "" || (len(s) > 1 && s[0] == '0') {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// Newer reports whether a is a strictly newer semver than b, compared per component numerically; a
// prerelease sorts below its release. Either side malformed (e.g. "dev") is never newer.
func Newer(a, b string) bool {
	va, ok := parse(a)
	if !ok {
		return false
	}
	vb, ok := parse(b)
	if !ok {
		return false
	}
	return compare(va, vb) > 0
}

func compare(a, b version) int {
	for i := range a.core {
		if a.core[i] != b.core[i] {
			return cmp.Compare(a.core[i], b.core[i])
		}
	}
	switch {
	case a.pre == nil && b.pre == nil:
		return 0
	case a.pre == nil:
		return 1
	case b.pre == nil:
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		if c := comparePre(a.pre[i], b.pre[i]); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(a.pre), len(b.pre))
}

// comparePre orders prerelease identifiers: numeric ones numerically and below alphanumeric ones, which
// compare as strings.
func comparePre(a, b string) int {
	na, aNum := number(a)
	nb, bNum := number(b)
	switch {
	case aNum && bNum:
		return cmp.Compare(na, nb)
	case aNum:
		return -1
	case bNum:
		return 1
	}
	return strings.Compare(a, b)
}
