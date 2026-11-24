package update

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/chriha/lanes/internal/app"
)

// Release is one published GitHub release: its tag and the raw release-notes body.
type Release struct {
	Tag  string
	Body string
}

// GitHubNotes returns a fetcher for the newest published releases (30 at most). Drafts and prereleases
// are skipped.
func GitHubNotes() func(ctx context.Context) ([]Release, error) {
	client := &http.Client{Timeout: Timeout}
	url := "https://api.github.com/repos/" + repo + "/releases?per_page=30"
	return func(ctx context.Context) ([]Release, error) { return fetchNotes(ctx, client, url) }
}

func fetchNotes(ctx context.Context, client *http.Client, url string) ([]Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("releases: %s", resp.Status)
	}
	var raw []struct {
		Tag        string `json:"tag_name"`
		Body       string `json:"body"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&raw); err != nil {
		return nil, fmt.Errorf("releases: %w", err)
	}
	var out []Release
	for _, r := range raw {
		if !r.Draft && !r.Prerelease {
			out = append(out, Release{Tag: r.Tag, Body: r.Body})
		}
	}
	return out, nil
}

var (
	shaPrefix    = regexp.MustCompile(`^([*-]\s+)[0-9a-f]{7,40}: `)
	authorSuffix = regexp.MustCompile(`\s+\(@[^)]+\)\s*$`)
)

// Clean tidies a goreleaser release body for display: the "## Changelog" heading is dropped, the group
// headings stay, and each bullet loses its "<sha>: " prefix and trailing "(@author)". Anything else is
// kept as is, so an unexpected format degrades to the raw lines.
func Clean(body string) string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		line = strings.TrimRight(line, " \t")
		if strings.EqualFold(line, "## Changelog") {
			continue
		}
		line = shaPrefix.ReplaceAllString(line, "$1")
		line = authorSuffix.ReplaceAllString(line, "")
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// Between returns the releases with last < tag <= current, newest first, at most max. Tags that are not
// semver are skipped; an invalid last or current returns nothing.
func Between(rels []Release, last, current string, max int) []Release {
	if !valid(last) || !valid(current) {
		return nil
	}
	var out []Release
	for _, r := range rels {
		if valid(r.Tag) && Newer(r.Tag, last) && !Newer(r.Tag, current) {
			out = append(out, r)
		}
	}
	slices.SortStableFunc(out, func(a, b Release) int {
		va, _ := parse(a.Tag)
		vb, _ := parse(b.Tag)
		return cmp.Compare(0, compare(va, vb))
	})
	if len(out) > max {
		out = out[:max]
	}
	return out
}

// LastRun returns the version recorded at path by SaveLastRun, and false when the file is missing or
// does not hold a semver tag.
func LastRun(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	v := strings.TrimSpace(string(b))
	return v, valid(v)
}

// SaveLastRun records v, the version that last ran, at path.
func SaveLastRun(path, v string) error {
	if err := os.MkdirAll(filepath.Dir(path), app.DirMode); err != nil {
		return err
	}
	return app.WriteFile(path, []byte(v+"\n"))
}
