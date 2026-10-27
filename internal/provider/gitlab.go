package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/chriha/lanes/internal/item"
)

// GitLab lists open MRs through `glab api graphql`, which reuses glab's auth.
type GitLab struct{ Project string }

const gqlQuery = `query($p:ID!){
  currentUser{username}
  project(fullPath:$p){ mergeRequests(state:opened,first:100,sort:UPDATED_DESC){ nodes{
    iid title webUrl draft approved sourceBranch targetBranch conflicts divergedFromTargetBranch
    author{username}
    assignees{nodes{username}}
    reviewers{nodes{username}}
    labels{nodes{title}}
    approvedBy{nodes{username}}
    headPipeline{status}
    resolvableDiscussionsCount resolvedDiscussionsCount
  }}}
}`

// Discussions are by far the slowest part (~9s vs ~2s), so they are fetched separately.
const gqlThreads = `query($p:ID!){
  project(fullPath:$p){ mergeRequests(state:opened,first:100,sort:UPDATED_DESC){ nodes{
    iid
    discussions(first:100){nodes{resolvable resolved notes(first:1){nodes{author{username}}}}}
  }}}
}`

type user struct{ Username string }
type nodes[T any] struct{ Nodes []T }

type gqlMR struct {
	IID                        string
	Title                      string
	WebURL                     string
	Draft                      bool
	Approved                   bool
	SourceBranch               string
	TargetBranch               string
	Conflicts                  bool
	DivergedFromTargetBranch   bool
	Author                     user
	Assignees                  nodes[user]
	Reviewers                  nodes[user]
	Labels                     nodes[struct{ Title string }]
	ApprovedBy                 nodes[user]
	HeadPipeline               *struct{ Status string }
	ResolvableDiscussionsCount int
	ResolvedDiscussionsCount   int
}

type gqlThreadMR struct {
	IID         string
	Discussions nodes[struct {
		Resolvable bool
		Resolved   bool
		Notes      nodes[struct{ Author user }]
	}]
}

type threadInfo struct {
	open int
	byMe bool
}

type gqlResp struct {
	Data struct {
		CurrentUser user
		Project     *struct{ MergeRequests nodes[gqlMR] }
	}
	Errors []struct{ Message string }
}

var ticketRe = regexp.MustCompile(`(?i)[a-z]+-\d+`)

func (g *GitLab) project() (string, error) {
	if g.Project != "" {
		return g.Project, nil
	}
	u, err := originPath()
	if err != nil {
		return "", fmt.Errorf("gitlab: set provider.project (cannot detect git remote)")
	}
	g.Project = u
	return u, nil
}

// originURL returns the URL of the current directory's origin remote.
func originURL() (string, error) {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// originPath returns the repo path (group/project, owner/repo) of the origin remote's URL.
func originPath() (string, error) {
	u, err := originURL()
	if err != nil {
		return "", err
	}
	_, path := splitRemote(u)
	return path, nil
}

func (g *GitLab) graphql(ctx context.Context, query, proj string, into any) error {
	out, err := exec.CommandContext(ctx, "glab", "api", "graphql", "-f", "query="+query, "-F", "p="+proj).Output()
	if err != nil {
		return fmt.Errorf("glab: %w", err)
	}
	if err := json.Unmarshal(out, into); err != nil {
		return fmt.Errorf("gitlab: bad response: %w", err)
	}
	return nil
}

func (g *GitLab) base(ctx context.Context, proj string) ([]gqlMR, string, error) {
	var r gqlResp
	if err := g.graphql(ctx, gqlQuery, proj, &r); err != nil {
		return nil, "", err
	}
	if len(r.Errors) > 0 {
		return nil, "", fmt.Errorf("gitlab: %s", r.Errors[0].Message)
	}
	if r.Data.Project == nil {
		return nil, "", fmt.Errorf("gitlab: project %q not found", proj)
	}
	return r.Data.Project.MergeRequests.Nodes, r.Data.CurrentUser.Username, nil
}

// Quick returns MRs without thread authorship (open_thread_by_me is inherited from the previous snapshot).
func (g *GitLab) Quick(ctx context.Context) ([]item.Item, error) {
	proj, err := g.project()
	if err != nil {
		return nil, err
	}
	mrs, me, err := g.base(ctx, proj)
	if err != nil {
		return nil, err
	}
	items := make([]item.Item, 0, len(mrs))
	for _, mr := range mrs {
		it := toItem(mr, me, nil)
		it.Pending = []string{"open_thread_by_me"}
		items = append(items, it)
	}
	return items, nil
}

func (g *GitLab) Fetch(ctx context.Context) ([]item.Item, error) {
	proj, err := g.project()
	if err != nil {
		return nil, err
	}
	var (
		mrs      []gqlMR
		me       string
		baseErr  error
		threads  gqlThreadsResp
		threadEr error
		done     = make(chan struct{})
	)
	go func() {
		defer close(done)
		threadEr = g.graphql(ctx, gqlThreads, proj, &threads)
	}()
	mrs, me, baseErr = g.base(ctx, proj)
	<-done
	if baseErr != nil {
		return nil, baseErr
	}
	if threadEr != nil {
		return nil, threadEr
	}
	if threads.Data.Project == nil {
		return nil, fmt.Errorf("gitlab: project %q not found", proj)
	}
	info := map[string]threadInfo{}
	for _, mr := range threads.Data.Project.MergeRequests.Nodes {
		var ti threadInfo
		// ponytail: first 100 discussions only, paginate if an MR ever exceeds that
		for _, d := range mr.Discussions.Nodes {
			if !d.Resolvable || d.Resolved {
				continue
			}
			ti.open++
			if len(d.Notes.Nodes) > 0 && d.Notes.Nodes[0].Author.Username == me {
				ti.byMe = true
			}
		}
		info[mr.IID] = ti
	}
	items := make([]item.Item, 0, len(mrs))
	for _, mr := range mrs {
		ti := info[mr.IID]
		items = append(items, toItem(mr, me, &ti))
	}
	return items, nil
}

type gqlThreadsResp struct {
	Data struct {
		Project *struct{ MergeRequests nodes[gqlThreadMR] }
	}
}

func names(u []user) []string {
	s := make([]string, 0, len(u))
	for _, x := range u {
		s = append(s, x.Username)
	}
	return s
}

func toItem(mr gqlMR, me string, ti *threadInfo) item.Item {
	labels := make([]string, 0, len(mr.Labels.Nodes))
	for _, l := range mr.Labels.Nodes {
		labels = append(labels, l.Title)
	}
	approvedBy := names(mr.ApprovedBy.Nodes)
	approvedByMe := false
	for _, a := range approvedBy {
		approvedByMe = approvedByMe || a == me
	}
	openThreads, openByMe := mr.ResolvableDiscussionsCount-mr.ResolvedDiscussionsCount, false
	if ti != nil {
		openThreads, openByMe = ti.open, ti.byMe
	}
	ci := ""
	if mr.HeadPipeline != nil {
		ci = strings.ToLower(mr.HeadPipeline.Status)
	}
	title := strings.TrimPrefix(strings.TrimPrefix(mr.Title, "Draft: "), "draft: ")
	ticket := strings.ToUpper(ticketRe.FindString(mr.Title))
	if ticket == "" {
		ticket = strings.ToUpper(ticketRe.FindString(mr.SourceBranch))
	}

	var badges []item.Badge
	if mr.Draft {
		badges = append(badges, item.Badge{Text: "draft", Kind: "dim"})
	}
	switch ci {
	case "failed":
		badges = append(badges, item.Badge{Text: "✗ ci", Kind: "bad"})
	case "success":
		badges = append(badges, item.Badge{Text: "✓ ci", Kind: "ok"})
	case "running", "pending", "created", "preparing", "waiting_for_resource":
		badges = append(badges, item.Badge{Text: "◌ ci", Kind: "warn"})
	}
	if mr.DivergedFromTargetBranch {
		badges = append(badges, item.Badge{Text: "↓ behind", Kind: "warn"})
	}
	if mr.Conflicts {
		badges = append(badges, item.Badge{Text: "⚠ conflict", Kind: "bad"})
	}
	if openThreads > 0 {
		badges = append(badges, item.Badge{Text: fmt.Sprintf("⚑ %d", openThreads), Kind: "warn"})
	}
	if mr.Approved || len(approvedBy) > 0 {
		badges = append(badges, item.Badge{Text: "✔ approved", Kind: "ok"})
	}

	return item.Item{
		ID:     "!" + mr.IID,
		Title:  title,
		URL:    mr.WebURL,
		Author: mr.Author.Username,
		Draft:  mr.Draft,
		Badges: badges,
		Fields: map[string]any{
			"me":                me,
			"labels":            labels,
			"approved":          mr.Approved || len(approvedBy) > 0,
			"approved_by":       approvedBy,
			"approved_by_me":    approvedByMe,
			"open_threads":      openThreads,
			"open_thread_by_me": openByMe,
			"ci":                ci,
			"conflicts":         mr.Conflicts,
			"diverged":          mr.DivergedFromTargetBranch,
			"reviewers":         names(mr.Reviewers.Nodes),
			"assignees":         names(mr.Assignees.Nodes),
			"branch":            mr.SourceBranch,
			"target":            mr.TargetBranch,
			"ticket":            ticket,
			"iid":               mr.IID,
		},
	}
}
