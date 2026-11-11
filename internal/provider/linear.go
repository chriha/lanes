package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/chriha/lanes/internal/item"
)

// linearTokenEnv is the env var the linear provider reads its API key from when token_env is not set.
const linearTokenEnv = "LINEAR_API_KEY"

const linearEndpoint = "https://api.linear.app/graphql"

// Linear lists a Linear team's open issues as tickets through the Linear GraphQL API.
type Linear struct {
	Team     string       // team key, e.g. ENG
	TokenEnv string       // env var holding the API key; empty = LINEAR_API_KEY
	Endpoint string       // GraphQL endpoint; empty = the Linear API
	Client   *http.Client // nil = http.DefaultClient
}

// linearQuery fetches one page of 100 open issues, most recently updated first, each with up to 50 relations and 50
// inverse relations; Fetch follows pageInfo until the last page.
const linearQuery = `query($team:String!,$after:String){
  viewer{displayName}
  issues(filter:{team:{key:{eq:$team}},state:{type:{nin:["completed","canceled"]}}},first:100,after:$after,orderBy:updatedAt){
    pageInfo{hasNextPage endCursor}
    nodes{
    identifier title url priority
    creator{displayName}
    assignee{displayName}
    state{name}
    labels(first:50){nodes{name}}
    relations(first:50){nodes{type relatedIssue{identifier state{type}}}}
    inverseRelations(first:50){nodes{type issue{identifier state{type}}}}
  }}
}`

type linearUser struct{ DisplayName string }

type linearIssue struct {
	Identifier string
	Title      string
	URL        string
	Priority   float64 // 0 none, 1 urgent … 4 low
	Creator    *linearUser
	Assignee   *linearUser
	State      struct{ Name string }
	Labels     nodes[struct{ Name string }]
	// Relations are the ones this issue holds to others; type "blocks" means it blocks that issue.
	Relations nodes[struct {
		Type         string
		RelatedIssue linearRef
	}]
	// InverseRelations are the relations other issues hold to this one; type "blocks" means that issue blocks it.
	InverseRelations nodes[struct {
		Type  string
		Issue linearRef
	}]
}

// linearRef is a linked issue.
type linearRef struct {
	Identifier string
	State      struct{ Type string }
}

type linearResp struct {
	Viewer linearUser
	Issues struct {
		PageInfo struct {
			HasNextPage bool
			EndCursor   string
		}
		Nodes []linearIssue
	}
}

// TokenVar is the name of the env var holding the API key.
func (l *Linear) TokenVar() string {
	if l.TokenEnv == "" {
		return linearTokenEnv
	}
	return l.TokenEnv
}

func (l *Linear) Fetch(ctx context.Context) ([]item.Item, error) {
	if l.Team == "" {
		return nil, fmt.Errorf("linear: team is not set (provider.team, e.g. \"ENG\")")
	}
	var items []item.Item
	me := ""
	after := ""
	for {
		vars := map[string]any{"team": l.Team}
		if after != "" {
			vars["after"] = after
		}
		var r linearResp
		if err := l.query(ctx, linearQuery, vars, &r); err != nil {
			return nil, err
		}
		me = r.Viewer.DisplayName
		for _, is := range r.Issues.Nodes {
			items = append(items, linearToItem(is, me))
		}
		pi := r.Issues.PageInfo
		if !pi.HasNextPage || pi.EndCursor == "" || pi.EndCursor == after {
			break
		}
		after = pi.EndCursor
	}
	if items == nil {
		items = []item.Item{}
	}
	return items, nil
}

// query runs a GraphQL query with vars and decodes the response's data into data.
func (l *Linear) query(ctx context.Context, query string, vars map[string]any, data any) error {
	env := l.TokenVar()
	token := os.Getenv(env)
	if token == "" {
		return fmt.Errorf("linear: %s is not set (a Linear API key)", env)
	}
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return fmt.Errorf("linear: %w", err)
	}
	endpoint := l.Endpoint
	if endpoint == "" {
		endpoint = linearEndpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("linear: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	client := l.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("linear: %w", err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("linear: read response: %w", err)
	}
	r := struct {
		Data   any // decodes into data
		Errors []struct{ Message string }
	}{Data: data}
	jsonErr := json.Unmarshal(out, &r)
	// Linear reports GraphQL errors with HTTP 400 too; prefer their message over the status.
	if len(r.Errors) > 0 && resp.StatusCode != http.StatusUnauthorized {
		return fmt.Errorf("linear: %s", r.Errors[0].Message)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("linear: unauthorized (HTTP 401), check %s", env)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("linear: HTTP %d", resp.StatusCode)
	case jsonErr != nil:
		return fmt.Errorf("linear: bad response: %w", jsonErr)
	}
	return nil
}

// linearToItem maps an issue onto the ticket field set (kind "ticket"); only blockers that are not done block it.
func linearToItem(is linearIssue, me string) item.Item {
	assignees := []string{}
	if is.Assignee != nil {
		assignees = append(assignees, is.Assignee.DisplayName)
	}
	labels := make([]string, 0, len(is.Labels.Nodes))
	for _, lb := range is.Labels.Nodes {
		labels = append(labels, lb.Name)
	}
	links := newRelLinks()
	add := func(rel string, r linearRef) {
		links.add(rel, r.Identifier, r.State.Type != "completed" && r.State.Type != "canceled")
	}
	for _, rel := range is.InverseRelations.Nodes {
		switch rel.Type {
		case "blocks":
			add("blocked_by", rel.Issue)
		case "related":
			add("related", rel.Issue)
		}
	}
	for _, rel := range is.Relations.Nodes {
		switch rel.Type {
		case "blocks":
			add("blocking", rel.RelatedIssue)
		case "related":
			add("related", rel.RelatedIssue)
		}
	}
	blockedBy := links.blockedBy
	status := strings.ToLower(is.State.Name)
	var badges []item.Badge
	if len(blockedBy) > 0 {
		badges = append(badges, item.Badge{Text: "blocked", Kind: "bad"})
	}
	if status != "" {
		badges = append(badges, item.Badge{Text: status, Kind: "info"})
	}
	author := ""
	if is.Creator != nil {
		author = is.Creator.DisplayName
	}
	return item.Item{
		ID:     is.Identifier,
		Title:  is.Title,
		URL:    is.URL,
		Author: author,
		Badges: badges,
		Fields: links.fields(map[string]any{
			"kind": "ticket", "key": is.Identifier, "status": status, "assignees": assignees, "labels": labels,
			"blocked": len(blockedBy) > 0, "priority": int(is.Priority), "me": me,
		}),
	}
}

// linearDetailQuery loads one issue's detail for the detail screen, never with the list poll.
// ponytail: first 100 comments only, replies listed flat; paginate if an issue ever exceeds that
const linearDetailQuery = `query($id:String!){
  issue(id:$id){ description comments(first:100){nodes{body createdAt user{displayName}}} }
}`

// Detail fetches an issue's description and comments, oldest comment first.
func (l *Linear) Detail(ctx context.Context, it item.Item) (item.Detail, error) {
	var r struct {
		Issue *struct {
			Description string
			Comments    nodes[struct {
				Body      string
				CreatedAt time.Time
				User      *linearUser // null for integrations
			}]
		}
	}
	if err := l.query(ctx, linearDetailQuery, map[string]any{"id": it.ID}, &r); err != nil {
		return item.Detail{}, err
	}
	if r.Issue == nil {
		return item.Detail{}, fmt.Errorf("linear: issue %s not found", it.ID)
	}
	d := item.Detail{Description: r.Issue.Description}
	for _, c := range r.Issue.Comments.Nodes {
		cm := item.Comment{Body: c.Body, Created: c.CreatedAt}
		if c.User != nil {
			cm.Author = c.User.DisplayName
		}
		d.Comments = append(d.Comments, cm)
	}
	slices.SortStableFunc(d.Comments, func(a, b item.Comment) int { return a.Created.Compare(b.Created) })
	return d, nil
}
