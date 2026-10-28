package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/chriha/lanes/internal/item"
)

// TestFieldsMatchFetchedItems keeps the declared field keys (shown in the help) in step with the items each built-in
// provider produces from its test fixture.
func TestFieldsMatchFetchedItems(t *testing.T) {
	tests := []struct {
		typ   string
		fetch func(t *testing.T) ([]item.Item, error)
	}{
		{"gitlab", func(t *testing.T) ([]item.Item, error) {
			// GitLab's Fetch shells out to glab; it builds every item with toItem, so decode a node and convert it.
			var mr gqlMR
			if err := json.Unmarshal([]byte(`{"iid":"7","title":"ABC-1 x","sourceBranch":"fix/abc-1","targetBranch":"main"}`), &mr); err != nil {
				return nil, err
			}
			return []item.Item{toItem(mr, "me", &threadInfo{open: 1, byMe: true})}, nil
		}},
		{"github", func(t *testing.T) ([]item.Item, error) {
			var args []string
			return (&GitHub{Project: "o/r", Run: fakeGH(ghResponse, nil, &args)}).Fetch(context.Background())
		}},
		{"github-issues", func(t *testing.T) ([]item.Item, error) {
			var args []string
			return (&GitHubIssues{Project: "o/r", Run: fakeGH(ghIssuesResponse, nil, &args)}).Fetch(context.Background())
		}},
		{"linear", func(t *testing.T) ([]item.Item, error) {
			t.Setenv("LINEAR_API_KEY", "lin_api_secret")
			srv := linearServer(t, http.StatusOK, linearResponse, nil, nil)
			return (&Linear{Team: "ENG", Endpoint: srv.URL, Client: srv.Client()}).Fetch(context.Background())
		}},
	}
	for _, tt := range tests {
		t.Run(tt.typ, func(t *testing.T) {
			items, err := tt.fetch(t)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) == 0 {
				t.Fatal("fixture produced no items")
			}
			want := Fields(tt.typ)
			for i, it := range items {
				got := make([]string, 0, len(it.Fields))
				for k := range it.Fields {
					got = append(got, k)
				}
				slices.Sort(got)
				if !reflect.DeepEqual(got, want) {
					t.Errorf("item %d keys = %v, declared Fields(%q) = %v", i, got, tt.typ, want)
				}
			}
		})
	}
}

func TestFieldsWithoutDeclaredKeys(t *testing.T) {
	for _, typ := range []string{"command", "auto", "nope", ""} {
		if got := Fields(typ); got != nil {
			t.Errorf("Fields(%q) = %v, want nil", typ, got)
		}
	}
}
