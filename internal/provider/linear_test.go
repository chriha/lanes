package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/item"
)

// A trimmed Linear GraphQL response: one assigned issue blocked by an open and a completed issue (plus a non-blocking
// relation), and one unassigned issue without a creator whose only blocker was canceled.
const linearResponse = `{"data":{"viewer":{"displayName":"me"},"issues":{"nodes":[
{"identifier":"ENG-1","title":"Add a thing","url":"https://linear.app/acme/issue/ENG-1","priority":2,
 "creator":{"displayName":"alex"},"assignee":{"displayName":"me"},"state":{"name":"In Progress"},
 "labels":{"nodes":[{"name":"feature"},{"name":"planning"}]},
 "inverseRelations":{"nodes":[
  {"type":"blocks","issue":{"identifier":"ENG-3","state":{"type":"started"}}},
  {"type":"blocks","issue":{"identifier":"ENG-4","state":{"type":"completed"}}},
  {"type":"related","issue":{"identifier":"ENG-5","state":{"type":"unstarted"}}}]},
 "relations":{"nodes":[
  {"type":"blocks","relatedIssue":{"identifier":"ENG-7","state":{"type":"backlog"}}},
  {"type":"blocks","relatedIssue":{"identifier":"ENG-8","state":{"type":"canceled"}}},
  {"type":"related","relatedIssue":{"identifier":"ENG-9","state":{"type":"started"}}},
  {"type":"related","relatedIssue":{"identifier":"ENG-10","state":{"type":"completed"}}},
  {"type":"duplicate","relatedIssue":{"identifier":"ENG-11","state":{"type":"started"}}}]}},
{"identifier":"ENG-2","title":"Idea","url":"https://linear.app/acme/issue/ENG-2","priority":0,
 "creator":null,"assignee":null,"state":{"name":"Todo"},
 "labels":{"nodes":[]},
 "relations":{"nodes":[]},
 "inverseRelations":{"nodes":[{"type":"blocks","issue":{"identifier":"ENG-6","state":{"type":"canceled"}}}]}}
]}}}`

// linearServer answers every request with status and body and records the last request's auth header and body.
func linearServer(t *testing.T, status int, body string, auth *string, req *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if auth != nil {
			*auth = r.Header.Get("Authorization")
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		if req != nil {
			if err := json.Unmarshal(b, req); err != nil {
				t.Errorf("request body is not JSON: %v", err)
			}
		}
		w.WriteHeader(status)
		if _, err := io.WriteString(w, body); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLinearFetch(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "lin_api_secret")
	var auth string
	var req map[string]any
	srv := linearServer(t, http.StatusOK, linearResponse, &auth, &req)

	items, err := (&Linear{Team: "ENG", Endpoint: srv.URL, Client: srv.Client()}).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if auth != "lin_api_secret" {
		t.Errorf("Authorization = %q, want the key from LINEAR_API_KEY", auth)
	}
	if vars, _ := req["variables"].(map[string]any); vars["team"] != "ENG" {
		t.Errorf("variables = %v, want team ENG", req["variables"])
	}
	query, _ := req["query"].(string)
	for _, want := range []string{"viewer", "issues(", "inverseRelations", "relations(first:50)", `nin:["completed","canceled"]`} {
		if !strings.Contains(query, want) {
			t.Errorf("query missing %q:\n%s", want, query)
		}
	}

	want := []item.Item{
		{ID: "ENG-1", Title: "Add a thing", URL: "https://linear.app/acme/issue/ENG-1", Author: "alex",
			Badges: []item.Badge{{Text: "blocked", Kind: "bad"}, {Text: "in progress", Kind: "info"}},
			Fields: map[string]any{"kind": "ticket", "key": "ENG-1", "status": "in progress", "assignees": []string{"me"},
				"labels": []string{"feature", "planning"}, "blocked": true, "blocked_by": []string{"ENG-3"}, "blocking": []string{"ENG-7"},
				"related": []string{"ENG-5", "ENG-9"}, "closed_links": []string{"blocked_by:ENG-4", "blocking:ENG-8", "related:ENG-10"},
				"priority": 2, "me": "me"}},
		{ID: "ENG-2", Title: "Idea", URL: "https://linear.app/acme/issue/ENG-2",
			Badges: []item.Badge{{Text: "todo", Kind: "info"}},
			Fields: map[string]any{"kind": "ticket", "key": "ENG-2", "status": "todo", "assignees": []string{},
				"labels": []string{}, "blocked": false, "blocked_by": []string{}, "blocking": []string{}, "related": []string{},
				"closed_links": []string{"blocked_by:ENG-6"}, "priority": 0, "me": "me"}},
	}
	if !reflect.DeepEqual(items, want) {
		t.Errorf("items:\n got %#v\nwant %#v", items, want)
	}
}

func TestLinearFetchTokenEnv(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "")
	t.Setenv("MY_LINEAR_KEY", "lin_api_other")
	var auth string
	srv := linearServer(t, http.StatusOK, `{"data":{"viewer":{"displayName":"me"},"issues":{"nodes":[]}}}`, &auth, nil)
	items, err := (&Linear{Team: "ENG", TokenEnv: "MY_LINEAR_KEY", Endpoint: srv.URL, Client: srv.Client()}).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if auth != "lin_api_other" || len(items) != 0 {
		t.Errorf("Authorization = %q, items = %v; want the key from MY_LINEAR_KEY and no items", auth, items)
	}
}

func TestLinearFetchErrors(t *testing.T) {
	const key = "lin_api_secret"
	for _, tc := range []struct {
		name, team, tokenEnv, token string
		status                      int
		body                        string
		want                        string
	}{
		{"missing team", "", "", key, http.StatusOK, `{}`, "linear: team is not set"},
		{"unset token", "ENG", "UNSET_LINEAR_KEY", "", http.StatusOK, `{}`, "linear: UNSET_LINEAR_KEY is not set"},
		{"empty token", "ENG", "", "", http.StatusOK, `{}`, "linear: LINEAR_API_KEY is not set"},
		{"unauthorized", "ENG", "", key, http.StatusUnauthorized, `{"errors":[{"message":"bad key ` + key + `"}]}`, "linear: unauthorized (HTTP 401), check LINEAR_API_KEY"},
		{"server error", "ENG", "", key, http.StatusInternalServerError, `oops`, "linear: HTTP 500"},
		{"graphql error", "ENG", "", key, http.StatusOK, `{"errors":[{"message":"Field 'nope' doesn't exist"}]}`, "linear: Field 'nope' doesn't exist"},
		{"bad json", "ENG", "", key, http.StatusOK, `nope`, "linear: bad response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LINEAR_API_KEY", tc.token)
			srv := linearServer(t, tc.status, tc.body, nil, nil)
			_, err := (&Linear{Team: tc.team, TokenEnv: tc.tokenEnv, Endpoint: srv.URL, Client: srv.Client()}).Fetch(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
			if strings.Contains(err.Error(), key) {
				t.Errorf("err %q leaks the API key", err)
			}
		})
	}
}

func TestNewLinear(t *testing.T) {
	p, err := New(config.Provider{Type: "linear", Team: "ENG", TokenEnv: "MY_KEY"})
	if err != nil {
		t.Fatal(err)
	}
	if l, ok := p.(*Linear); !ok || l.Team != "ENG" || l.TokenEnv != "MY_KEY" {
		t.Fatalf("New = %#v, want *Linear{Team: ENG, TokenEnv: MY_KEY}", p)
	}
}
