package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/chriha/lanes/internal/item"
)

// GitHubIssues lists a repository's open issues as tickets through `gh api graphql`.
type GitHubIssues struct {
	Project string // owner/repo; detected from the origin remote when empty
	// Run executes gh with args; nil runs the real gh binary.
	Run func(ctx context.Context, args ...string) ([]byte, error)
}

// ponytail: first 100 open issues (and 20 blockers, blocked issues and sub-issues each); paginate if a repo needs more.
const ghIssuesQueryFmt = `query($owner:String!,$name:String!){
  viewer{login}
  repository(owner:$owner,name:$name){ issues(states:OPEN,first:100,orderBy:{field:UPDATED_AT,direction:DESC}){ nodes{
    number title url
    author{login}
    assignees(first:20){nodes{login}}
    labels(first:50){nodes{name}}
    %s
  }}}
}`

// ghIssuesRelations are the fields of hosts that support issue dependencies and sub-issues; older hosts reject them.
const ghIssuesRelations = `blockedBy(first:20){nodes{number state}}
    blocking(first:20){nodes{number state}}
    parent{number state}
    subIssues(first:20){nodes{number state}}`

var (
	ghIssuesQuery           = fmt.Sprintf(ghIssuesQueryFmt, ghIssuesRelations)
	ghIssuesQueryNoRelation = fmt.Sprintf(ghIssuesQueryFmt, "")
)

// ghIssuesUnknownRelation reports whether the response rejects one of the relation fields as unknown.
func ghIssuesUnknownRelation(out []byte) bool {
	var r ghIssuesResp
	if json.Unmarshal(out, &r) != nil {
		return false
	}
	for _, e := range r.Errors {
		if !strings.Contains(e.Message, "doesn't exist on type") {
			continue
		}
		for _, f := range []string{"blockedBy", "blocking", "parent", "subIssues"} {
			if strings.Contains(e.Message, "'"+f+"'") {
				return true
			}
		}
	}
	return false
}

type ghIssue struct {
	Number    int
	Title     string
	URL       string
	Author    *ghUser // null for deleted users
	Assignees nodes[ghUser]
	Labels    nodes[struct{ Name string }]
	BlockedBy nodes[ghRef]
	Blocking  nodes[ghRef]
	Parent    *ghRef // null without a parent
	SubIssues nodes[ghRef]
}

// ghRef is a linked issue.
type ghRef struct {
	Number int
	State  string
}

type ghIssuesResp struct {
	Data struct {
		Viewer     ghUser
		Repository *struct{ Issues nodes[ghIssue] }
	}
	Errors []struct{ Message string }
}

func (g *GitHubIssues) Fetch(ctx context.Context) ([]item.Item, error) {
	out, err := ghGraphQL(ctx, "github-issues", &g.Project, g.Run, ghIssuesQuery)
	if ghIssuesUnknownRelation(out) {
		// The host has no issue dependencies or sub-issues: list the issues without relations.
		out, err = ghGraphQL(ctx, "github-issues", &g.Project, g.Run, ghIssuesQueryNoRelation)
	}
	if err != nil {
		return nil, err
	}
	var r ghIssuesResp
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("github-issues: bad response: %w", err)
	}
	if len(r.Errors) > 0 {
		return nil, fmt.Errorf("github-issues: %s", r.Errors[0].Message)
	}
	if r.Data.Repository == nil {
		return nil, fmt.Errorf("github-issues: repository %q not found", g.Project)
	}
	me := r.Data.Viewer.Login
	issues := r.Data.Repository.Issues.Nodes
	items := make([]item.Item, 0, len(issues))
	for _, is := range issues {
		items = append(items, ghIssueToItem(is, me))
	}
	return items, nil
}

// ghIssueToItem maps an issue onto the ticket field set (kind "ticket"); only open blockers block it.
func ghIssueToItem(is ghIssue, me string) item.Item {
	assignees := make([]string, 0, len(is.Assignees.Nodes))
	for _, a := range is.Assignees.Nodes {
		assignees = append(assignees, a.Login)
	}
	labels := make([]string, 0, len(is.Labels.Nodes))
	for _, l := range is.Labels.Nodes {
		labels = append(labels, l.Name)
	}
	links := newRelLinks()
	add := func(rel string, r ghRef) { links.add(rel, "#"+strconv.Itoa(r.Number), r.State == "OPEN") }
	for _, b := range is.BlockedBy.Nodes {
		add("blocked_by", b)
	}
	for _, b := range is.Blocking.Nodes {
		add("blocking", b)
	}
	// GitHub has no "related" relation; the parent and sub-issues are the closest equivalent.
	if is.Parent != nil {
		add("related", *is.Parent)
	}
	for _, c := range is.SubIssues.Nodes {
		add("related", c)
	}
	blockedBy := links.blockedBy
	var badges []item.Badge
	if len(blockedBy) > 0 {
		badges = append(badges, item.Badge{Text: "blocked", Kind: "bad"})
	}
	author := ""
	if is.Author != nil {
		author = is.Author.Login
	}
	return item.Item{
		ID:     "#" + strconv.Itoa(is.Number),
		Title:  is.Title,
		URL:    is.URL,
		Author: author,
		Badges: badges,
		Fields: links.fields(map[string]any{
			"kind": "ticket", "number": is.Number, "status": "open", "assignees": assignees, "labels": labels,
			"blocked": len(blockedBy) > 0, "priority": 0, "me": me,
		}),
	}
}
