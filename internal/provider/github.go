package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/chriha/lanes/internal/item"
)

// GitHub lists open PRs through `gh api graphql`, which reuses gh's auth.
type GitHub struct {
	Project string // owner/repo; detected from the origin remote when empty
	// Run executes gh with args; nil runs the real gh binary.
	Run func(ctx context.Context, args ...string) ([]byte, error)

	mu      sync.Mutex
	authors map[string]string // review thread id -> login of its first commenter; a thread's first author never changes
}

// ghThreadAuthorsQuery looks up the first commenter of review threads by id (1 point per batch).
const ghThreadAuthorsQuery = `query($ids:[ID!]!){
  nodes(ids:$ids){ ... on PullRequestReviewThread{ id comments(first:1){nodes{author{login}}} } }
}`

// ghAuthorBatch is the most ids one nodes(ids:) lookup accepts.
const ghAuthorBatch = 100

// ponytail: review threads carry only id and isResolved here; first-comment authors are looked up once per thread.
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
    reviewThreads(first:100){nodes{id isResolved}}
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
		ID         string
		IsResolved bool
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
	authors := g.threadAuthors(ctx, prs)
	for _, pr := range prs {
		it := ghToItem(pr, me, authors)
		for _, t := range pr.ReviewThreads.Nodes {
			if _, ok := authors[t.ID]; !t.IsResolved && !ok {
				it.Pending = []string{"open_thread_by_me"} // inherit the previous value, like gitlab's Quick
				break
			}
		}
		items = append(items, it)
	}
	return items, nil
}

// threadAuthors returns the cached first-comment authors, first fetching those of open threads not cached yet.
// A failed lookup is not an error: the threads stay unknown and the caller marks their items pending.
func (g *GitHub) threadAuthors(ctx context.Context, prs []ghPR) map[string]string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.authors == nil {
		g.authors = map[string]string{}
	}
	var missing []string
	queued := map[string]bool{}
	for _, pr := range prs {
		for _, t := range pr.ReviewThreads.Nodes {
			if _, ok := g.authors[t.ID]; !t.IsResolved && !ok && !queued[t.ID] {
				queued[t.ID] = true
				missing = append(missing, t.ID)
			}
		}
	}
	for len(missing) > 0 {
		n := min(len(missing), ghAuthorBatch)
		if !g.lookupAuthors(ctx, missing[:n]) {
			break
		}
		missing = missing[n:]
	}
	return maps.Clone(g.authors)
}

// lookupAuthors fetches the authors of ids into the cache, reporting whether the lookup succeeded.
func (g *GitHub) lookupAuthors(ctx context.Context, ids []string) bool {
	args := []string{"api", "graphql", "-f", "query=" + ghThreadAuthorsQuery}
	for _, id := range ids {
		args = append(args, "-f", "ids[]="+id)
	}
	out, err := ghRunner(g.Run)(ctx, args...)
	if err != nil {
		return false
	}
	var r struct {
		Data struct {
			Nodes []*struct {
				ID       string
				Comments nodes[struct{ Author ghUser }]
			}
		}
		Errors []struct{ Message string }
	}
	if json.Unmarshal(out, &r) != nil || len(r.Errors) > 0 {
		return false
	}
	for _, n := range r.Data.Nodes {
		if n != nil && len(n.Comments.Nodes) > 0 {
			g.authors[n.ID] = n.Comments.Nodes[0].Author.Login
		}
	}
	return true
}

// ghRunner returns run, or a runner of the real gh binary when run is nil.
func ghRunner(run func(context.Context, ...string) ([]byte, error)) func(context.Context, ...string) ([]byte, error) {
	if run != nil {
		return run
	}
	return func(ctx context.Context, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, "gh", args...).Output()
	}
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
	out, err := ghRunner(run)(ctx, "api", "graphql", "-f", "query="+query, "-f", "owner="+owner, "-f", "name="+name)
	if err != nil {
		return nil, fmt.Errorf("gh: %w", err)
	}
	return out, nil
}

// ghToItem maps a PR onto gitlab's MR shape so both providers share fields and badges.
func ghToItem(pr ghPR, me string, authors map[string]string) item.Item {
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
		if a, ok := authors[t.ID]; ok && a == me {
			ti.byMe = true
		}
	}
	it := toItem(mr, me, &ti)
	it.ID = "#" + mr.IID
	delete(it.Fields, "iid")
	it.Fields["number"] = pr.Number
	return it
}
