package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/chriha/lanes/internal/item"
)

// GitHub lists open PRs through `gh api graphql`, which reuses gh's auth.
type GitHub struct {
	Project string // owner/repo; detected from the origin remote when empty
	// Run executes gh with args; nil runs the real gh binary.
	Run func(ctx context.Context, args ...string) ([]byte, error)
}

// ponytail: one query including review threads; split into a Quick pass like gitlab if threads make it slow.
const ghQuery = `query($owner:String!,$name:String!){
  viewer{login}
  repository(owner:$owner,name:$name){ pullRequests(states:OPEN,first:100,orderBy:{field:UPDATED_AT,direction:DESC}){ nodes{
    number title url isDraft headRefName baseRefName mergeable mergeStateStatus reviewDecision
    author{login}
    assignees(first:20){nodes{login}}
    reviewRequests(first:20){nodes{requestedReviewer{... on User{login}}}}
    labels(first:50){nodes{name}}
    latestReviews(first:50){nodes{state author{login}}}
    commits(last:1){nodes{commit{statusCheckRollup{state}}}}
    reviewThreads(first:100){nodes{isResolved comments(first:1){nodes{author{login}}}}}
  }}}
}`

type ghUser struct{ Login string }

type ghPR struct {
	Number           int
	Title            string
	URL              string
	IsDraft          bool
	HeadRefName      string
	BaseRefName      string
	Mergeable        string
	MergeStateStatus string
	ReviewDecision   string
	Author           ghUser
	Assignees        nodes[ghUser]
	ReviewRequests   nodes[struct{ RequestedReviewer ghUser }]
	Labels           nodes[struct{ Name string }]
	LatestReviews    nodes[struct {
		State  string
		Author ghUser
	}]
	Commits nodes[struct {
		Commit struct{ StatusCheckRollup *struct{ State string } }
	}]
	ReviewThreads nodes[struct {
		IsResolved bool
		Comments   nodes[struct{ Author ghUser }]
	}]
}

type ghResp struct {
	Data struct {
		Viewer     ghUser
		Repository *struct{ PullRequests nodes[ghPR] }
	}
	Errors []struct{ Message string }
}

func (g *GitHub) Fetch(ctx context.Context) ([]item.Item, error) {
	out, err := ghGraphQL(ctx, "github", &g.Project, g.Run, ghQuery)
	if err != nil {
		return nil, err
	}
	var r ghResp
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("github: bad response: %w", err)
	}
	if len(r.Errors) > 0 {
		return nil, fmt.Errorf("github: %s", r.Errors[0].Message)
	}
	if r.Data.Repository == nil {
		return nil, fmt.Errorf("github: repository %q not found", g.Project)
	}
	me := r.Data.Viewer.Login
	prs := r.Data.Repository.PullRequests.Nodes
	items := make([]item.Item, 0, len(prs))
	for _, pr := range prs {
		items = append(items, ghToItem(pr, me))
	}
	return items, nil
}

// ghGraphQL runs query through gh for the owner/repo in *project, which is detected from the origin remote
// (and stored) when empty; typ prefixes the errors. run nil runs the real gh binary.
func ghGraphQL(ctx context.Context, typ string, project *string, run func(context.Context, ...string) ([]byte, error), query string) ([]byte, error) {
	if *project == "" {
		u, err := originPath()
		if err != nil {
			return nil, fmt.Errorf("%s: set provider.project (cannot detect git remote)", typ)
		}
		*project = u
	}
	owner, name, ok := strings.Cut(*project, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return nil, fmt.Errorf("%s: project %q is not owner/repo", typ, *project)
	}
	if run == nil {
		run = func(ctx context.Context, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, "gh", args...).Output()
		}
	}
	out, err := run(ctx, "api", "graphql", "-f", "query="+query, "-f", "owner="+owner, "-f", "name="+name)
	if err != nil {
		return nil, fmt.Errorf("gh: %w", err)
	}
	return out, nil
}

// ghToItem maps a PR onto gitlab's MR shape so both providers share fields and badges.
func ghToItem(pr ghPR, me string) item.Item {
	mr := gqlMR{
		IID:                      strconv.Itoa(pr.Number),
		Title:                    pr.Title,
		WebURL:                   pr.URL,
		Draft:                    pr.IsDraft,
		Approved:                 pr.ReviewDecision == "APPROVED",
		SourceBranch:             pr.HeadRefName,
		TargetBranch:             pr.BaseRefName,
		Conflicts:                pr.Mergeable == "CONFLICTING",
		DivergedFromTargetBranch: pr.MergeStateStatus == "BEHIND",
		Author:                   user{pr.Author.Login},
	}
	for _, a := range pr.Assignees.Nodes {
		mr.Assignees.Nodes = append(mr.Assignees.Nodes, user{a.Login})
	}
	// Reviewers: pending requests plus everyone who already reviewed (GitHub drops the request once reviewed).
	seen := map[string]bool{}
	addReviewer := func(login string) {
		if login != "" && !seen[login] {
			seen[login] = true
			mr.Reviewers.Nodes = append(mr.Reviewers.Nodes, user{login})
		}
	}
	for _, r := range pr.ReviewRequests.Nodes {
		addReviewer(r.RequestedReviewer.Login) // empty for team requests
	}
	for _, r := range pr.LatestReviews.Nodes {
		addReviewer(r.Author.Login)
		if r.State == "APPROVED" {
			mr.ApprovedBy.Nodes = append(mr.ApprovedBy.Nodes, user{r.Author.Login})
		}
	}
	for _, l := range pr.Labels.Nodes {
		mr.Labels.Nodes = append(mr.Labels.Nodes, struct{ Title string }{l.Name})
	}
	if c := pr.Commits.Nodes; len(c) > 0 && c[0].Commit.StatusCheckRollup != nil {
		ci := map[string]string{"SUCCESS": "success", "FAILURE": "failed", "ERROR": "failed", "PENDING": "running", "EXPECTED": "running"}
		mr.HeadPipeline = &struct{ Status string }{ci[c[0].Commit.StatusCheckRollup.State]}
	}
	// ponytail: first 100 threads only, paginate if a PR ever exceeds that
	var ti threadInfo
	for _, t := range pr.ReviewThreads.Nodes {
		if t.IsResolved {
			continue
		}
		ti.open++
		if len(t.Comments.Nodes) > 0 && t.Comments.Nodes[0].Author.Login == me {
			ti.byMe = true
		}
	}
	it := toItem(mr, me, &ti)
	it.ID = "#" + mr.IID
	delete(it.Fields, "iid")
	it.Fields["number"] = pr.Number
	return it
}
