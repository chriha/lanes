package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

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

// ponytail: the 100 most recently updated open issues (and 50 relations and 50 inverse relations each); paginate if a team needs more.
const linearQuery = `query($team:String!){
  viewer{displayName}
  issues(filter:{team:{key:{eq:$team}},state:{type:{nin:["completed","canceled"]}}},first:100,orderBy:updatedAt){ nodes{
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
	Data struct {
		Viewer linearUser
		Issues nodes[linearIssue]
	}
	Errors []struct{ Message string }
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
	env := l.TokenVar()
	token := os.Getenv(env)
	if token == "" {
		return nil, fmt.Errorf("linear: %s is not set (a Linear API key)", env)
	}
	body, err := json.Marshal(map[string]any{"query": linearQuery, "variables": map[string]string{"team": l.Team}})
	if err != nil {
		return nil, fmt.Errorf("linear: %w", err)
	}
	endpoint := l.Endpoint
	if endpoint == "" {
		endpoint = linearEndpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("linear: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	client := l.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("linear: %w", err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("linear: read response: %w", err)
	}
	var r linearResp
	jsonErr := json.Unmarshal(out, &r)
	// Linear reports GraphQL errors with HTTP 400 too; prefer their message over the status.
	if len(r.Errors) > 0 && resp.StatusCode != http.StatusUnauthorized {
		return nil, fmt.Errorf("linear: %s", r.Errors[0].Message)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, fmt.Errorf("linear: unauthorized (HTTP 401), check %s", env)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("linear: HTTP %d", resp.StatusCode)
	case jsonErr != nil:
		return nil, fmt.Errorf("linear: bad response: %w", jsonErr)
	}
	me := r.Data.Viewer.DisplayName
	items := make([]item.Item, 0, len(r.Data.Issues.Nodes))
	for _, is := range r.Data.Issues.Nodes {
		items = append(items, linearToItem(is, me))
	}
	return items, nil
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
