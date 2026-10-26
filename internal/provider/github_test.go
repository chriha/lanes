package provider

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/filter"
	"github.com/chriha/lanes/internal/item"
)

// A trimmed `gh api graphql` response: one approved PR with an open thread by me and failing CI, one draft PR
// approved only through a review, behind its target and in conflict, with pending checks.
const ghResponse = `{"data":{"viewer":{"login":"me"},"repository":{"pullRequests":{"nodes":[
{"number":12,"title":"ABC-7 add thing","url":"https://github.com/o/r/pull/12","isDraft":false,
 "headRefName":"feature/abc-7","baseRefName":"main","mergeable":"MERGEABLE","mergeStateStatus":"CLEAN",
 "reviewDecision":"APPROVED","author":{"login":"alex"},
 "assignees":{"nodes":[{"login":"alex"}]},
 "reviewRequests":{"nodes":[{"requestedReviewer":{"login":"sam"}},{"requestedReviewer":{}}]},
 "labels":{"nodes":[{"name":"waiting-for-review"}]},
 "latestReviews":{"nodes":[{"state":"APPROVED","author":{"login":"me"}},{"state":"COMMENTED","author":{"login":"kim"}}]},
 "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE"}}}]},
 "reviewThreads":{"nodes":[
   {"isResolved":false,"comments":{"nodes":[{"author":{"login":"me"}}]}},
   {"isResolved":true,"comments":{"nodes":[{"author":{"login":"kim"}}]}}]}},
{"number":13,"title":"wip","url":"https://github.com/o/r/pull/13","isDraft":true,
 "headRefName":"wip","baseRefName":"main","mergeable":"CONFLICTING","mergeStateStatus":"BEHIND",
 "reviewDecision":null,"author":{"login":"me"},
 "assignees":{"nodes":[]},"reviewRequests":{"nodes":[]},"labels":{"nodes":[]},
 "latestReviews":{"nodes":[{"state":"APPROVED","author":{"login":"kim"}}]},
 "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"PENDING"}}}]},
 "reviewThreads":{"nodes":[]}}
]}}}}`

func fakeGH(out string, err error, gotArgs *[]string) func(context.Context, ...string) ([]byte, error) {
	return func(_ context.Context, args ...string) ([]byte, error) {
		*gotArgs = args
		return []byte(out), err
	}
}

func TestGitHubFetch(t *testing.T) {
	var args []string
	items, err := (&GitHub{Project: "o/r", Run: fakeGH(ghResponse, nil, &args)}).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"api graphql", "owner=o", "name=r", "states:OPEN"} {
		if !strings.Contains(joined, want) {
			t.Errorf("gh args %q missing %q", joined, want)
		}
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}

	a := items[0]
	if a.ID != "#12" || a.Title != "ABC-7 add thing" || a.URL != "https://github.com/o/r/pull/12" || a.Author != "alex" || a.Draft {
		t.Errorf("item 0 = %+v", a)
	}
	wantFields := map[string]any{
		"me": "me", "labels": []string{"waiting-for-review"}, "approved": true, "approved_by": []string{"me"},
		"approved_by_me": true, "open_threads": 1, "open_thread_by_me": true, "ci": "failed", "conflicts": false,
		"diverged": false, "reviewers": []string{"sam", "me", "kim"}, "assignees": []string{"alex"},
		"branch": "feature/abc-7", "target": "main", "ticket": "ABC-7", "number": 12,
	}
	if !reflect.DeepEqual(a.Fields, wantFields) {
		t.Errorf("item 0 fields:\n got %#v\nwant %#v", a.Fields, wantFields)
	}
	wantBadges := []item.Badge{{Text: "✗ ci", Kind: "bad"}, {Text: "⚑ 1", Kind: "warn"}, {Text: "✔ approved", Kind: "ok"}}
	if !reflect.DeepEqual(a.Badges, wantBadges) {
		t.Errorf("item 0 badges = %v, want %v", a.Badges, wantBadges)
	}

	b := items[1]
	if b.ID != "#13" || !b.Draft {
		t.Errorf("item 1 = %+v", b)
	}
	for k, want := range map[string]any{
		"approved": true, "approved_by": []string{"kim"}, "approved_by_me": false, "ci": "running",
		"conflicts": true, "diverged": true, "open_threads": 0, "open_thread_by_me": false,
	} {
		if !reflect.DeepEqual(b.Fields[k], want) {
			t.Errorf("item 1 %s = %#v, want %#v", k, b.Fields[k], want)
		}
	}
	wantBadges = []item.Badge{{Text: "draft", Kind: "dim"}, {Text: "◌ ci", Kind: "warn"},
		{Text: "↓ behind", Kind: "warn"}, {Text: "⚠ conflict", Kind: "bad"}, {Text: "✔ approved", Kind: "ok"}}
	if !reflect.DeepEqual(b.Badges, wantBadges) {
		t.Errorf("item 1 badges = %v, want %v", b.Badges, wantBadges)
	}

	f, err := filter.Compile(`"waiting-for-review" in labels && author != me && open_thread_by_me`)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []bool{true, false} {
		if got, err := f.Match(items[i].Env()); err != nil || got != want {
			t.Errorf("filter on item %d: got %v err %v, want %v", i, got, err, want)
		}
	}
}

func TestGitHubCIMapping(t *testing.T) {
	for state, want := range map[string]string{
		"SUCCESS": "success", "FAILURE": "failed", "ERROR": "failed", "PENDING": "running", "EXPECTED": "running",
	} {
		resp := strings.Replace(ghResponse, `"FAILURE"`, `"`+state+`"`, 1)
		var args []string
		items, err := (&GitHub{Project: "o/r", Run: fakeGH(resp, nil, &args)}).Fetch(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if got := items[0].Fields["ci"]; got != want {
			t.Errorf("%s: ci = %v, want %s", state, got, want)
		}
	}
}

func TestGitHubFetchErrors(t *testing.T) {
	for _, tc := range []struct {
		name, project, out string
		runErr             error
		want               string
	}{
		{"gh fails", "o/r", "", errors.New("exit 1"), "gh: exit 1"},
		{"graphql error", "o/r", `{"errors":[{"message":"Could not resolve"}]}`, nil, "Could not resolve"},
		{"repo missing", "o/r", `{"data":{"viewer":{"login":"me"},"repository":null}}`, nil, `"o/r" not found`},
		{"bad json", "o/r", `nope`, nil, "bad response"},
		{"bad project", "noslash", `{}`, nil, "owner/repo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var args []string
			_, err := (&GitHub{Project: tc.project, Run: fakeGH(tc.out, tc.runErr, &args)}).Fetch(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestNewGitHub(t *testing.T) {
	p, err := New(config.Provider{Type: "github", Project: "o/r"})
	if err != nil {
		t.Fatal(err)
	}
	if g, ok := p.(*GitHub); !ok || g.Project != "o/r" {
		t.Fatalf("New = %#v, want *GitHub{Project: o/r}", p)
	}
}
