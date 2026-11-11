package provider

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/filter"
	"github.com/chriha/lanes/internal/item"
)

// A trimmed `gh api graphql` response: one approved PR with an open thread by me and failing CI, one draft PR
// approved only through a review, behind its target and in conflict, with pending checks. The first closes an
// issue of its own repository (#5), one of another repository (#9) and one of its repository spelled in other case (#7).
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
   {"id":"T1","isResolved":false},
   {"id":"T2","isResolved":true}]},
 "closingIssuesReferences":{"nodes":[
   {"number":5,"repository":{"nameWithOwner":"o/r"}},
   {"number":9,"repository":{"nameWithOwner":"other/r"}},
   {"number":7,"repository":{"nameWithOwner":"O/R"}}]}},
{"number":13,"title":"wip","url":"https://github.com/o/r/pull/13","isDraft":true,
 "headRefName":"wip","baseRefName":"main","mergeable":"CONFLICTING","mergeStateStatus":"BEHIND",
 "reviewDecision":null,"author":{"login":"me"},
 "assignees":{"nodes":[]},"reviewRequests":{"nodes":[]},"labels":{"nodes":[]},
 "latestReviews":{"nodes":[{"state":"APPROVED","author":{"login":"kim"}}]},
 "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"PENDING"}}}]},
 "reviewThreads":{"nodes":[]}}
]}}}}`

// ghThreadAuthors answers the author lookup for the threads in ghResponse.
const ghThreadAuthors = `{"data":{"nodes":[{"id":"T1","comments":{"nodes":[{"author":{"login":"me"}}]}}]}}`

// fakeGH answers the PR query with out and records its args in gotArgs; author lookups (ids[] args) get
// ghThreadAuthors.
func fakeGH(out string, err error, gotArgs *[]string) func(context.Context, ...string) ([]byte, error) {
	return func(_ context.Context, args ...string) ([]byte, error) {
		if strings.Contains(strings.Join(args, " "), "ids[]=") {
			return []byte(ghThreadAuthors), nil
		}
		*gotArgs = args
		return []byte(out), err
	}
}

// lookups returns the thread ids of each author lookup in calls.
func lookups(calls [][]string) [][]string {
	var out [][]string
	for _, args := range calls {
		var ids []string
		for _, a := range args {
			if id, ok := strings.CutPrefix(a, "ids[]="); ok {
				ids = append(ids, id)
			}
		}
		if ids != nil {
			out = append(out, ids)
		}
	}
	return out
}

func TestGitHubThreadAuthorsIncremental(t *testing.T) {
	body := func(threads string) string {
		return `{"data":{"viewer":{"login":"me"},"repository":{"pullRequests":{"nodes":[{"number":1,"title":"t","url":"u","reviewThreads":{"nodes":[` +
			threads + `]}}]}}}}`
	}
	one := `{"id":"T1","isResolved":false},{"id":"T9","isResolved":true}`
	two := one + `,{"id":"T3","isResolved":false}`

	var calls [][]string
	var cur string
	lookupOut := ghThreadAuthors
	lookupErr := error(nil)
	g := &GitHub{Project: "o/r", Run: func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, args)
		if strings.Contains(strings.Join(args, " "), "ids[]=") {
			return []byte(lookupOut), lookupErr
		}
		return []byte(cur), nil
	}}
	fetch := func(threads string) item.Item {
		t.Helper()
		cur = body(threads)
		calls = nil
		items, err := g.Fetch(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return items[0]
	}

	it := fetch(one)
	if got, want := lookups(calls), [][]string{{"T1"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("first fetch lookups = %v, want %v (resolved T9 must not be looked up)", got, want)
	}
	if it.Fields["open_thread_by_me"] != true || it.Fields["open_threads"] != 1 || it.Pending != nil {
		t.Errorf("first fetch: by_me=%v open=%v pending=%v", it.Fields["open_thread_by_me"], it.Fields["open_threads"], it.Pending)
	}

	it = fetch(one)
	if got := lookups(calls); got != nil {
		t.Errorf("second fetch lookups = %v, want none", got)
	}
	if it.Fields["open_thread_by_me"] != true {
		t.Errorf("second fetch: open_thread_by_me = %v, want true from cache", it.Fields["open_thread_by_me"])
	}

	lookupOut = `{"data":{"nodes":[{"id":"T3","comments":{"nodes":[{"author":{"login":"kim"}}]}}]}}`
	it = fetch(two)
	if got, want := lookups(calls), [][]string{{"T3"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("new thread lookups = %v, want %v", got, want)
	}
	if it.Fields["open_threads"] != 2 || it.Fields["open_thread_by_me"] != true {
		t.Errorf("new thread: open=%v by_me=%v", it.Fields["open_threads"], it.Fields["open_thread_by_me"])
	}
}

func TestGitHubThreadAuthorLookupBatches(t *testing.T) {
	var threads []string
	for i := range 250 {
		threads = append(threads, fmt.Sprintf(`{"id":"T%d","isResolved":false}`, i))
	}
	prs := `{"data":{"viewer":{"login":"me"},"repository":{"pullRequests":{"nodes":[{"number":1,"title":"t","url":"u","reviewThreads":{"nodes":[` +
		strings.Join(threads, ",") + `]}}]}}}}`
	var calls [][]string
	g := &GitHub{Project: "o/r", Run: func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, args)
		if strings.Contains(strings.Join(args, " "), "ids[]=") {
			return []byte(`{"data":{"nodes":[]}}`), nil
		}
		return []byte(prs), nil
	}}
	if _, err := g.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	var sizes []int
	for _, ids := range lookups(calls) {
		sizes = append(sizes, len(ids))
	}
	if want := []int{100, 100, 50}; !reflect.DeepEqual(sizes, want) {
		t.Errorf("lookup batch sizes = %v, want %v", sizes, want)
	}
}

func TestGitHubThreadAuthorLookupFailure(t *testing.T) {
	for name, lookup := range map[string]func() ([]byte, error){
		"run error":     func() ([]byte, error) { return nil, errors.New("exit 1") },
		"graphql error": func() ([]byte, error) { return []byte(`{"errors":[{"message":"boom"}]}`), nil },
		"bad json":      func() ([]byte, error) { return []byte(`nope`), nil },
	} {
		t.Run(name, func(t *testing.T) {
			g := &GitHub{Project: "o/r", Run: func(_ context.Context, args ...string) ([]byte, error) {
				if strings.Contains(strings.Join(args, " "), "ids[]=") {
					return lookup()
				}
				return []byte(ghResponse), nil
			}}
			items, err := g.Fetch(context.Background())
			if err != nil {
				t.Fatalf("Fetch err = %v, want the lookup failure swallowed", err)
			}
			if len(items) != 2 || !reflect.DeepEqual(items[0].Pending, []string{"open_thread_by_me"}) {
				t.Errorf("item 0 pending = %v, want [open_thread_by_me]", items[0].Pending)
			}
			if items[0].Fields["open_threads"] != 1 || items[1].Pending != nil {
				t.Errorf("open_threads = %v, item 1 pending = %v", items[0].Fields["open_threads"], items[1].Pending)
			}
		})
	}
}

func TestGitHubFetch(t *testing.T) {
	var args []string
	items, err := (&GitHub{Project: "o/r", Run: fakeGH(ghResponse, nil, &args)}).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"api graphql", "owner=o", "name=r", "states:OPEN", "closingIssuesReferences(first:10)"} {
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
		"branch": "feature/abc-7", "target": "main", "ticket": "ABC-7", "number": 12, "kind": "pr",
		"closes": []string{"#5", "#7"},
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
		"conflicts": true, "diverged": true, "open_threads": 0, "open_thread_by_me": false, "closes": []string{},
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

func TestGitHubCIJobs(t *testing.T) {
	if !strings.Contains(ghQuery, "contexts(") {
		t.Error("ghQuery does not request the status check contexts")
	}
	for _, tc := range []struct {
		name     string
		contexts string
		want     []string
	}{
		{"failing check runs and status contexts", `[
			{"__typename":"CheckRun","name":"lint","conclusion":"FAILURE"},
			{"__typename":"CheckRun","name":"slow","conclusion":"TIMED_OUT"},
			{"__typename":"CheckRun","name":"boot","conclusion":"STARTUP_FAILURE"},
			{"__typename":"StatusContext","context":"ci/legacy","state":"ERROR"},
			{"__typename":"StatusContext","context":"ci/other","state":"FAILURE"}]`,
			[]string{"✗ lint", "✗ slow", "✗ boot", "✗ ci/legacy", "✗ ci/other"}},
		{"others are ignored", `[
			{"__typename":"CheckRun","name":"a","conclusion":"SUCCESS"},
			{"__typename":"CheckRun","name":"b","conclusion":"SKIPPED"},
			{"__typename":"CheckRun","name":"c","conclusion":"CANCELLED"},
			{"__typename":"CheckRun","name":"d","conclusion":"NEUTRAL"},
			{"__typename":"CheckRun","name":"e","conclusion":null},
			{"__typename":"StatusContext","context":"f","state":"PENDING"},
			{"__typename":"StatusContext","context":"g","state":"SUCCESS"}]`, nil},
		{"no contexts", `[]`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := strings.Replace(ghResponse, `"statusCheckRollup":{"state":"FAILURE"}`,
				`"statusCheckRollup":{"state":"FAILURE","contexts":{"nodes":`+tc.contexts+`}}`, 1)
			var args []string
			items, err := (&GitHub{Project: "o/r", Run: fakeGH(resp, nil, &args)}).Fetch(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			got, _ := items[0].Fields["ci_jobs"].([]string)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ci_jobs = %#v, want %#v", got, tc.want)
			}
			if _, ok := items[1].Fields["ci_jobs"]; ok {
				t.Errorf("PR without contexts has ci_jobs = %v", items[1].Fields["ci_jobs"])
			}
		})
	}
}

const ghDetailResponse = `{"data":{"repository":{"pullRequest":{
 "body":"Fixes the thing.",
 "comments":{"nodes":[{"body":"looks good","createdAt":"2026-11-04T10:00:00Z","author":{"login":"kim"}}]},
 "reviewThreads":{"nodes":[
  {"isResolved":false,"path":"a.go","line":12,"originalLine":10,"comments":{"nodes":[
   {"body":"why?","url":"https://github.com/o/r/pull/7#discussion_r1","createdAt":"2026-11-01T10:00:00Z","author":{"login":"alex"}},
   {"body":"because","url":"https://github.com/o/r/pull/7#discussion_r2","createdAt":"2026-11-01T11:00:00Z","author":{"login":"me"}}]}},
  {"isResolved":true,"path":"b.go","line":null,"originalLine":7,"comments":{"nodes":[
   {"body":"removed line","url":"https://github.com/o/r/pull/7#discussion_r3","createdAt":"2026-11-02T10:00:00Z","author":{"login":"alex"}}]}}]},
 "commits":{"nodes":[{"commit":{"abbreviatedOid":"abc1234","statusCheckRollup":{"state":"FAILURE","contexts":{"nodes":[
  {"__typename":"CheckRun","name":"build","status":"COMPLETED","conclusion":"SUCCESS","startedAt":"2026-11-01T10:00:00Z","completedAt":"2026-11-01T10:00:30Z","checkSuite":{"workflowRun":{"workflow":{"name":"CI"}}}},
  {"__typename":"CheckRun","name":"lint","status":"COMPLETED","conclusion":"FAILURE","startedAt":"2026-11-01T10:00:05Z","completedAt":"2026-11-01T10:02:05Z","checkSuite":{"workflowRun":{"workflow":{"name":"CI"}}}},
  {"__typename":"CheckRun","name":"docs","status":"COMPLETED","conclusion":"SKIPPED","startedAt":null,"completedAt":null,"checkSuite":{"workflowRun":{"workflow":{"name":"CI"}}}},
  {"__typename":"CheckRun","name":"e2e","status":"IN_PROGRESS","conclusion":null,"startedAt":"2026-11-01T10:00:10Z","completedAt":null,"checkSuite":{"workflowRun":null}},
  {"__typename":"StatusContext","context":"ci/jenkins","state":"PENDING"}]}}}}]}
}}}}`

func TestGitHubDetail(t *testing.T) {
	at := func(s string) time.Time {
		tm, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return tm
	}
	want := item.Detail{
		Description: "Fixes the thing.",
		Pipeline: &item.Pipeline{ID: "abc1234", Status: "failed", Duration: 125 * time.Second, Jobs: []item.Job{
			{Name: "build", Stage: "CI", Status: "success", Duration: 30 * time.Second},
			{Name: "lint", Stage: "CI", Status: "failed", Duration: 120 * time.Second},
			{Name: "docs", Stage: "CI", Status: "skipped"},
			{Name: "e2e", Stage: "checks", Status: "running"},
			{Name: "ci/jenkins", Stage: "checks", Status: "pending"},
		}},
		Threads: []item.Thread{
			{File: "a.go", Line: 12, URL: "https://github.com/o/r/pull/7#discussion_r1", Comments: []item.Comment{
				{Author: "alex", Body: "why?", Created: at("2026-11-01T10:00:00Z")},
				{Author: "me", Body: "because", Created: at("2026-11-01T11:00:00Z")},
			}},
			{File: "b.go", Line: 7, Resolved: true, URL: "https://github.com/o/r/pull/7#discussion_r3", Comments: []item.Comment{
				{Author: "alex", Body: "removed line", Created: at("2026-11-02T10:00:00Z")},
			}},
		},
		Comments: []item.Comment{{Author: "kim", Body: "looks good", Created: at("2026-11-04T10:00:00Z")}},
	}
	for _, tc := range []struct {
		name       string
		it         item.Item
		resp       string
		runErr     error
		want       *item.Detail
		wantErr    string
		wantNumber string
	}{
		{"full PR", item.Item{ID: "#7", Fields: map[string]any{"number": 7}}, ghDetailResponse, nil, &want, "", "number=7"},
		{"no checks", item.Item{ID: "#8"}, `{"data":{"repository":{"pullRequest":{"body":"d","comments":{"nodes":[]},"reviewThreads":{"nodes":[]},"commits":{"nodes":[{"commit":{"abbreviatedOid":"x","statusCheckRollup":null}}]}}}}}`,
			nil, &item.Detail{Description: "d"}, "", "number=8"},
		{"bad number", item.Item{ID: "#x"}, ghDetailResponse, nil, nil, "bad pull request number", ""},
		{"run fails", item.Item{ID: "#7"}, "", errors.New("boom"), nil, "gh: boom", ""},
		{"graphql error", item.Item{ID: "#7"}, `{"errors":[{"message":"nope"}]}`, nil, nil, "github: nope", ""},
		{"PR missing", item.Item{ID: "#7"}, `{"data":{"repository":{"pullRequest":null}}}`, nil, nil, "not found", ""},
		{"repo missing", item.Item{ID: "#7"}, `{"data":{"repository":null}}`, nil, nil, "not found", ""},
		{"bad json", item.Item{ID: "#7"}, `nope`, nil, nil, "bad response", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls [][]string
			g := &GitHub{Project: "o/r", Run: func(_ context.Context, args ...string) ([]byte, error) {
				calls = append(calls, args)
				return []byte(tc.resp), tc.runErr
			}}
			got, err := g.Detail(context.Background(), tc.it)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, *tc.want) {
				t.Errorf("detail =\n%+v\nwant\n%+v", got, *tc.want)
			}
			if len(calls) != 1 || !slices.Contains(calls[0], tc.wantNumber) {
				t.Errorf("calls = %v, want one with %s", calls, tc.wantNumber)
			}
			if q := strings.Join(calls[0], " "); !strings.Contains(q, "url") {
				t.Errorf("query %q does not ask for the comment url", q)
			}
		})
	}
}

func TestGitHubListPollFetchesNoDetail(t *testing.T) {
	var args []string
	if _, err := (&GitHub{Project: "o/r", Run: fakeGH(ghResponse, nil, &args)}).Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if q := strings.Join(args, " "); strings.Contains(q, "body") || strings.Contains(q, "comments(last") {
		t.Errorf("list poll query fetches detail data: %s", q)
	}
}
