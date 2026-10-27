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

// A trimmed `gh api graphql` response: one issue assigned to me with an open and a closed blocker, one issue whose
// only blocker is closed, and one without assignees, labels or blockers.
const ghIssuesResponse = `{"data":{"viewer":{"login":"me"},"repository":{"issues":{"nodes":[
{"number":7,"title":"Add a thing","url":"https://github.com/o/r/issues/7","author":{"login":"alex"},
 "assignees":{"nodes":[{"login":"me"},{"login":"sam"}]},
 "labels":{"nodes":[{"name":"feature"},{"name":"planning"}]},
 "blockedBy":{"nodes":[{"number":3,"state":"OPEN"},{"number":4,"state":"CLOSED"}]}},
{"number":8,"title":"Fix a bug","url":"https://github.com/o/r/issues/8","author":{"login":"me"},
 "assignees":{"nodes":[{"login":"me"}]},
 "labels":{"nodes":[{"name":"bug"}]},
 "blockedBy":{"nodes":[{"number":5,"state":"CLOSED"}]}},
{"number":9,"title":"Idea","url":"https://github.com/o/r/issues/9","author":null,
 "assignees":{"nodes":[]},"labels":{"nodes":[]},"blockedBy":{"nodes":[]}}
]}}}}`

func TestGitHubIssuesFetch(t *testing.T) {
	var args []string
	items, err := (&GitHubIssues{Project: "o/r", Run: fakeGH(ghIssuesResponse, nil, &args)}).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"api graphql", "owner=o", "name=r", "issues(states:OPEN", "blockedBy"} {
		if !strings.Contains(joined, want) {
			t.Errorf("gh args %q missing %q", joined, want)
		}
	}

	want := []item.Item{
		{ID: "#7", Title: "Add a thing", URL: "https://github.com/o/r/issues/7", Author: "alex",
			Badges: []item.Badge{{Text: "blocked", Kind: "bad"}},
			Fields: map[string]any{"kind": "ticket", "number": 7, "status": "open", "assignees": []string{"me", "sam"},
				"labels": []string{"feature", "planning"}, "blocked": true, "blocked_by": []string{"#3"}, "priority": 0, "me": "me"}},
		{ID: "#8", Title: "Fix a bug", URL: "https://github.com/o/r/issues/8", Author: "me",
			Fields: map[string]any{"kind": "ticket", "number": 8, "status": "open", "assignees": []string{"me"},
				"labels": []string{"bug"}, "blocked": false, "blocked_by": []string{}, "priority": 0, "me": "me"}},
		{ID: "#9", Title: "Idea", URL: "https://github.com/o/r/issues/9",
			Fields: map[string]any{"kind": "ticket", "number": 9, "status": "open", "assignees": []string{},
				"labels": []string{}, "blocked": false, "blocked_by": []string{}, "priority": 0, "me": "me"}},
	}
	if !reflect.DeepEqual(items, want) {
		t.Errorf("items:\n got %#v\nwant %#v", items, want)
	}

	// The README's example view: my open, unblocked issues that are not being planned.
	f, err := filter.Compile(`me in assignees && !blocked && !("planning" in labels)`)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []bool{false, true, false} {
		if got, err := f.Match(items[i].Env()); err != nil || got != want {
			t.Errorf("filter on item %d: got %v err %v, want %v", i, got, err, want)
		}
	}
}

func TestGitHubIssuesFetchErrors(t *testing.T) {
	for _, tc := range []struct {
		name, project, out string
		runErr             error
		want               string
	}{
		{"gh fails", "o/r", "", errors.New("exit 1"), "gh: exit 1"},
		{"graphql error", "o/r", `{"errors":[{"message":"Could not resolve"}]}`, nil, "Could not resolve"},
		{"repo missing", "o/r", `{"data":{"viewer":{"login":"me"},"repository":null}}`, nil, `"o/r" not found`},
		{"bad json", "o/r", `nope`, nil, "bad response"},
		{"bad project", "noslash", `{}`, nil, `github-issues: project "noslash" is not owner/repo`},
		{"too many segments", "a/b/c", `{}`, nil, `github-issues: project "a/b/c" is not owner/repo`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var args []string
			_, err := (&GitHubIssues{Project: tc.project, Run: fakeGH(tc.out, tc.runErr, &args)}).Fetch(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestNewGitHubIssues(t *testing.T) {
	p, err := New(config.Provider{Type: "github-issues", Project: "o/r"})
	if err != nil {
		t.Fatal(err)
	}
	if g, ok := p.(*GitHubIssues); !ok || g.Project != "o/r" {
		t.Fatalf("New = %#v, want *GitHubIssues{Project: o/r}", p)
	}
}
