package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chriha/lanes/internal/item"
)

// GitLab lists open MRs through `glab api graphql`, which reuses glab's auth.
type GitLab struct {
	Project string
	// Run executes glab with args; nil runs the real glab binary.
	Run func(ctx context.Context, args ...string) ([]byte, error)

	mu   sync.Mutex
	snap map[string]mrSnap // last full result by iid; in memory only
}

// mrSnap is what Fetch remembers of an MR to tell whether its discussions need loading again.
type mrSnap struct {
	updatedAt  string
	resolvable int
	resolved   int
	threads    threadInfo
}

const gqlQuery = `query($p:ID!){
  currentUser{username}
  project(fullPath:$p){ mergeRequests(state:opened,first:100,sort:UPDATED_DESC){ nodes{
    iid title webUrl updatedAt draft approved sourceBranch targetBranch conflicts divergedFromTargetBranch
    author{username}
    assignees{nodes{username}}
    reviewers{nodes{username}}
    labels{nodes{title}}
    approvedBy{nodes{username}}
    headPipeline{status jobs(statuses:[FAILED],first:50){nodes{name status allowFailure}}}
    resolvableDiscussionsCount resolvedDiscussionsCount
  }}}
}`

// Discussions are by far the slowest part (~9s vs ~2s), so they are fetched separately and only for the MRs
// that changed; %s is the iids list, e.g. "1","2".
const gqlThreadsFmt = `query($p:ID!){
  project(fullPath:$p){ mergeRequests(iids:[%s],state:opened,first:100){ nodes{
    iid
    discussions(first:100){nodes{resolvable resolved notes(first:1){nodes{author{username}}}}}
  }}}
}`

type user struct{ Username string }
type nodes[T any] struct{ Nodes []T }

type gqlMR struct {
	IID                        string
	UpdatedAt                  string
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
	HeadPipeline               *gqlPipeline
	ResolvableDiscussionsCount int
	ResolvedDiscussionsCount   int
}

type gqlPipeline struct {
	Status string
	Jobs   nodes[gqlJob]
}

type gqlJob struct {
	Name         string
	Status       string
	AllowFailure bool
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
	run := g.Run
	if run == nil {
		run = func(ctx context.Context, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, "glab", args...).Output()
		}
	}
	out, err := run(ctx, "api", "graphql", "-f", "query="+query, "-F", "p="+proj)
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

// Fetch returns the full result. Discussions are loaded only for MRs that are new or whose updatedAt or discussion
// counts differ from the last full result; the others keep their previous thread info.
func (g *GitLab) Fetch(ctx context.Context) ([]item.Item, error) {
	proj, err := g.project()
	if err != nil {
		return nil, err
	}
	mrs, me, err := g.base(ctx, proj)
	if err != nil {
		return nil, err
	}
	g.mu.Lock()
	prev := g.snap
	g.mu.Unlock()

	info := map[string]threadInfo{}
	var changed []string
	for _, mr := range mrs {
		s, ok := prev[mr.IID]
		if ok && s.updatedAt == mr.UpdatedAt && s.resolvable == mr.ResolvableDiscussionsCount && s.resolved == mr.ResolvedDiscussionsCount {
			info[mr.IID] = s.threads
			continue
		}
		changed = append(changed, mr.IID)
	}
	if len(changed) > 0 {
		fresh, err := g.threads(ctx, proj, me, changed)
		if err != nil {
			return nil, err
		}
		for _, iid := range changed {
			info[iid] = fresh[iid]
		}
	}

	snap := make(map[string]mrSnap, len(mrs))
	items := make([]item.Item, 0, len(mrs))
	for _, mr := range mrs {
		ti := info[mr.IID]
		snap[mr.IID] = mrSnap{mr.UpdatedAt, mr.ResolvableDiscussionsCount, mr.ResolvedDiscussionsCount, ti}
		items = append(items, toItem(mr, me, &ti))
	}
	g.mu.Lock()
	g.snap = snap
	g.mu.Unlock()
	return items, nil
}

// threads loads the discussions of the MRs with the given iids.
func (g *GitLab) threads(ctx context.Context, proj, me string, iids []string) (map[string]threadInfo, error) {
	quoted := make([]string, 0, len(iids))
	for _, iid := range iids {
		if _, err := strconv.ParseUint(iid, 10, 64); err != nil {
			return nil, fmt.Errorf("gitlab: bad merge request iid %q", iid)
		}
		quoted = append(quoted, strconv.Quote(iid))
	}
	var threads gqlThreadsResp
	if err := g.graphql(ctx, fmt.Sprintf(gqlThreadsFmt, strings.Join(quoted, ",")), proj, &threads); err != nil {
		return nil, err
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
	return info, nil
}

// gqlDetailFmt loads one MR's detail for the detail screen, never with the list poll; %s is the quoted iid.
const gqlDetailFmt = `query($p:ID!){
  project(fullPath:$p){ mergeRequest(iid:%s){
    description
    headPipeline{id status duration jobs(first:100){nodes{name status duration allowFailure stage{name}}}}
    discussions(first:100){nodes{resolvable resolved
      notes(first:100){nodes{body system url createdAt author{username} position{filePath newLine oldLine}}}}}
  }}
}`

type gqlNote struct {
	Body      string
	System    bool
	URL       string
	CreatedAt time.Time
	Author    user
	Position  *struct {
		FilePath         string
		NewLine, OldLine int
	}
}

type gqlDetailResp struct {
	Data struct {
		Project *struct {
			MergeRequest *struct {
				Description  string
				HeadPipeline *struct {
					ID       string
					Status   string
					Duration int
					Jobs     nodes[struct {
						Name         string
						Status       string
						Duration     int
						AllowFailure bool
						Stage        struct{ Name string }
					}]
				}
				Discussions nodes[struct {
					Resolvable bool
					Resolved   bool
					Notes      nodes[gqlNote]
				}]
			}
		}
	}
	Errors []struct{ Message string }
}

// Detail fetches an MR's description, head pipeline with all jobs, threads and comments. Resolvable discussions
// are threads; the other non-system notes are comments.
func (g *GitLab) Detail(ctx context.Context, it item.Item) (item.Detail, error) {
	proj, err := g.project()
	if err != nil {
		return item.Detail{}, err
	}
	iid, _ := it.Fields["iid"].(string)
	if iid == "" {
		iid = strings.TrimPrefix(it.ID, "!")
	}
	if _, err := strconv.ParseUint(iid, 10, 64); err != nil {
		return item.Detail{}, fmt.Errorf("gitlab: bad merge request iid %q", iid)
	}
	var r gqlDetailResp
	if err := g.graphql(ctx, fmt.Sprintf(gqlDetailFmt, strconv.Quote(iid)), proj, &r); err != nil {
		return item.Detail{}, err
	}
	if len(r.Errors) > 0 {
		return item.Detail{}, fmt.Errorf("gitlab: %s", r.Errors[0].Message)
	}
	if r.Data.Project == nil || r.Data.Project.MergeRequest == nil {
		return item.Detail{}, fmt.Errorf("gitlab: merge request !%s not found in %q", iid, proj)
	}
	mr := r.Data.Project.MergeRequest
	d := item.Detail{Description: mr.Description}
	if p := mr.HeadPipeline; p != nil {
		pl := &item.Pipeline{
			ID:       p.ID[strings.LastIndex(p.ID, "/")+1:], // gid://gitlab/Ci::Pipeline/123
			Status:   strings.ToLower(p.Status),
			Duration: time.Duration(p.Duration) * time.Second,
		}
		// ponytail: first 100 jobs only, paginate if a pipeline ever exceeds that
		for _, j := range p.Jobs.Nodes {
			status := strings.ToLower(j.Status)
			if status == "failed" && j.AllowFailure {
				status = "warning"
			}
			pl.Jobs = append(pl.Jobs, item.Job{Name: j.Name, Stage: j.Stage.Name, Status: status, Duration: time.Duration(j.Duration) * time.Second})
		}
		d.Pipeline = pl
	}
	for _, disc := range mr.Discussions.Nodes {
		var cs []item.Comment
		for _, n := range disc.Notes.Nodes {
			if !n.System {
				cs = append(cs, item.Comment{Author: n.Author.Username, Body: n.Body, Created: n.CreatedAt})
			}
		}
		if !disc.Resolvable {
			d.Comments = append(d.Comments, cs...)
			continue
		}
		th := item.Thread{Resolved: disc.Resolved, Comments: cs}
		if ns := disc.Notes.Nodes; len(ns) > 0 {
			th.URL = ns[0].URL
			if pos := ns[0].Position; pos != nil {
				th.File, th.Line = pos.FilePath, pos.NewLine
				if th.Line == 0 {
					th.Line = pos.OldLine
				}
			}
		}
		d.Threads = append(d.Threads, th)
	}
	return d, nil
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
	var ciJobs []string // failed jobs "✗ name", failed jobs allowed to fail "⚠ name"
	if mr.HeadPipeline != nil {
		ci = strings.ToLower(mr.HeadPipeline.Status)
		for _, j := range mr.HeadPipeline.Jobs.Nodes {
			switch {
			case j.Status != "FAILED":
			case j.AllowFailure:
				ciJobs = append(ciJobs, "⚠ "+j.Name)
			default:
				ciJobs = append(ciJobs, "✗ "+j.Name)
			}
		}
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

	it := item.Item{
		ID:     "!" + mr.IID,
		Title:  title,
		URL:    mr.WebURL,
		Author: mr.Author.Username,
		Draft:  mr.Draft,
		Badges: badges,
		Fields: map[string]any{
			"kind":              "pr",
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
	if len(ciJobs) > 0 {
		it.Fields["ci_jobs"] = ciJobs
	}
	return it
}
