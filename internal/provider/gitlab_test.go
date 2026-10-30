package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/chriha/lanes/internal/filter"
	"github.com/chriha/lanes/internal/item"
)

func TestToItemAndFilter(t *testing.T) {
	var mr gqlMR
	mr.IID, mr.Title, mr.SourceBranch = "7", "Draft: ABC-12 thing", "fix/abc-12"
	mr.Author.Username = "alex"
	mr.Labels.Nodes = append(mr.Labels.Nodes, struct{ Title string }{"waiting-for-review"})
	it := toItem(mr, "me", &threadInfo{open: 1, byMe: true})
	if it.Title != "ABC-12 thing" || it.Fields["ticket"] != "ABC-12" || it.Fields["open_thread_by_me"] != true {
		t.Fatalf("bad item: %+v", it)
	}
	if it.Fields["kind"] != "pr" {
		t.Errorf("kind = %#v, want \"pr\"", it.Fields["kind"])
	}
	for src, want := range map[string]bool{
		`"waiting-for-review" in labels && author != me`: true,
		`!open_thread_by_me`:                             false,
		`author == me`:                                   false,
	} {
		f, err := filter.Compile(src)
		if err != nil {
			t.Fatal(err)
		}
		got, err := f.Match(it.Env())
		if err != nil || got != want {
			t.Errorf("%s: got %v err %v, want %v", src, got, err, want)
		}
	}
}

func TestToItemDiverged(t *testing.T) {
	if !strings.Contains(gqlQuery, "divergedFromTargetBranch") {
		t.Error("gqlQuery does not request divergedFromTargetBranch")
	}
	behind := item.Badge{Text: "↓ behind", Kind: "warn"}
	for _, tc := range []struct {
		name      string
		diverged  bool
		conflicts bool
		want      []item.Badge
	}{
		{"in sync", false, false, []item.Badge{{Text: "✓ ci", Kind: "ok"}}},
		{"behind", true, false, []item.Badge{{Text: "✓ ci", Kind: "ok"}, behind}},
		{"behind with conflict", true, true, []item.Badge{{Text: "✓ ci", Kind: "ok"}, behind, {Text: "⚠ conflict", Kind: "bad"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Decode the GraphQL node shape so the test also covers the JSON field mapping.
			raw := fmt.Sprintf(`{"iid":"1","headPipeline":{"status":"SUCCESS"},"conflicts":%t,"divergedFromTargetBranch":%t}`, tc.conflicts, tc.diverged)
			var mr gqlMR
			if err := json.Unmarshal([]byte(raw), &mr); err != nil {
				t.Fatal(err)
			}
			it := toItem(mr, "me", nil)
			if !reflect.DeepEqual(it.Badges, tc.want) {
				t.Errorf("badges = %v, want %v", it.Badges, tc.want)
			}
			if it.Fields["diverged"] != tc.diverged {
				t.Errorf("diverged = %v, want %v", it.Fields["diverged"], tc.diverged)
			}
			f, err := filter.Compile(`diverged && ci == "success"`)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := f.Match(it.Env()); err != nil || got != tc.diverged {
				t.Errorf("filter: got %v err %v, want %v", got, err, tc.diverged)
			}
		})
	}
}

const glBaseResponse = `{"data":{"currentUser":{"username":"me"},"project":{"mergeRequests":{"nodes":[
 {"iid":"1","title":"ABC-1 one","webUrl":"u1","author":{"username":"alex"},"headPipeline":{"status":"SUCCESS"},
  "resolvableDiscussionsCount":2,"resolvedDiscussionsCount":0},
 {"iid":"2","title":"two","webUrl":"u2","author":{"username":"me"}}]}}}}`

const glThreadsResponse = `{"data":{"project":{"mergeRequests":{"nodes":[
 {"iid":"1","discussions":{"nodes":[
  {"resolvable":true,"resolved":false,"notes":{"nodes":[{"author":{"username":"me"}}]}},
  {"resolvable":true,"resolved":true,"notes":{"nodes":[{"author":{"username":"me"}}]}},
  {"resolvable":false,"resolved":false,"notes":{"nodes":[]}}]}}]}}}}`

// fakeGlab answers by query; Fetch runs both queries concurrently, so it keeps no shared state.
func fakeGlab(base, threads string, err error) func(context.Context, ...string) ([]byte, error) {
	return func(_ context.Context, args ...string) ([]byte, error) {
		if err != nil {
			return nil, err
		}
		if strings.Contains(strings.Join(args, " "), "discussions(") {
			return []byte(threads), nil
		}
		return []byte(base), nil
	}
}

func TestGitLabFetchAndQuick(t *testing.T) {
	g := &GitLab{Project: "g/p", Run: fakeGlab(glBaseResponse, glThreadsResponse, nil)}

	items, err := g.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "!1" || items[1].ID != "!2" {
		t.Fatalf("items = %+v", items)
	}
	if items[0].Fields["open_threads"] != 1 || items[0].Fields["open_thread_by_me"] != true {
		t.Errorf("threads not applied: %+v", items[0].Fields)
	}
	if items[1].Fields["open_threads"] != 0 || items[1].Fields["me"] != "me" {
		t.Errorf("bad item 2: %+v", items[1].Fields)
	}

	quick, err := g.Quick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(quick) != 2 || !reflect.DeepEqual(quick[0].Pending, []string{"open_thread_by_me"}) {
		t.Errorf("quick = %+v", quick)
	}
}

func TestGitLabFetchErrors(t *testing.T) {
	for _, tc := range []struct {
		name, base, threads string
		runErr              error
		want                string
	}{
		{"run fails", "", "", errors.New("boom"), "glab: boom"},
		{"bad json", "not json", glThreadsResponse, nil, "bad response"},
		{"graphql error", `{"errors":[{"message":"nope"}]}`, glThreadsResponse, nil, "gitlab: nope"},
		{"project missing", `{"data":{"project":null}}`, glThreadsResponse, nil, "not found"},
		{"threads project missing", glBaseResponse, `{"data":{"project":null}}`, nil, "not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &GitLab{Project: "g/p", Run: fakeGlab(tc.base, tc.threads, tc.runErr)}
			if _, err := g.Fetch(context.Background()); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}
