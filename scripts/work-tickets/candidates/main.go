// Command candidates prints the /work-tickets triage facts as JSON.
//
// Usage: go run ./scripts/work-tickets/candidates "[12>15 20 -37]"
//
// Issues and pull requests come from the gh CLI of the current repository.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

const planMarker = "<!-- work-tickets plan -->"

var priorityLabel = regexp.MustCompile(`^priority:([1-4])$`)

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}

func gh(args ...string) []byte {
	out, err := exec.Command("gh", args...).Output()
	if err != nil {
		msg := err.Error()
		if exitErr, ok := err.(*exec.ExitError); ok {
			msg = strings.TrimSpace(string(exitErr.Stderr))
		}
		fail("gh %s failed: %s", strings.Join(args, " "), msg)
	}

	return out
}

type issueJSON struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
	State     string `json:"state"`
	Labels    []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Assignees []struct {
		Login string `json:"login"`
	} `json:"assignees"`
	Comments []struct {
		Body string `json:"body"`
	} `json:"comments"`
}

type blockerJSON struct {
	Number int    `json:"number"`
	State  string `json:"state"`
}

type prJSON struct {
	URL  string `json:"url"`
	Head string `json:"headRefName"`
	Base string `json:"baseRefName"`
}

func fetchIssue(id int, me string, openPRs []prJSON) Ticket {
	var issue issueJSON
	raw := gh("issue", "view", strconv.Itoa(id), "--json", "number,title,body,createdAt,state,labels,assignees,comments")
	if err := json.Unmarshal(raw, &issue); err != nil {
		fail("cannot read issue #%d: %v", id, err)
	}

	t := Ticket{ID: id, Title: issue.Title, CreatedAt: issue.CreatedAt, HasPlan: strings.Contains(issue.Body, "## Plan")}

	for _, l := range issue.Labels {
		t.Labels = append(t.Labels, l.Name)
		if m := priorityLabel.FindStringSubmatch(l.Name); m != nil {
			t.Priority, _ = strconv.Atoi(m[1])
		}
	}
	for _, a := range issue.Assignees {
		if a.Login == me {
			t.AssignedToMe = true
		}
	}
	for _, c := range issue.Comments {
		if strings.HasPrefix(strings.TrimSpace(c.Body), planMarker) {
			t.HasPlan = true
		}
	}

	// A failing call must not read as "no blockers".
	var blockers []blockerJSON
	raw = gh("api", "--paginate", "--slurp", fmt.Sprintf("repos/{owner}/{repo}/issues/%d/dependencies/blocked_by", id))
	var pages [][]blockerJSON
	if err := json.Unmarshal(raw, &pages); err != nil {
		fail("cannot read blockers of #%d: %v", id, err)
	}
	for _, page := range pages {
		blockers = append(blockers, page...)
	}
	for _, b := range blockers {
		t.Blockers = append(t.Blockers, Blocker{ID: b.Number, Status: strings.ToLower(b.State), Resolved: strings.EqualFold(b.State, "closed")})
	}

	branch := regexp.MustCompile(`^[^/]+/` + strconv.Itoa(id) + `-`)
	for _, pr := range openPRs {
		if branch.MatchString(pr.Head) {
			t.OpenPR = &PR{URL: pr.URL, Source: pr.Head, Target: pr.Base}

			break
		}
	}

	return t
}

func main() {
	argument := ""
	if len(os.Args) > 1 {
		argument = os.Args[1]
	}

	parsed, err := ParseArgument(argument)
	if err != nil {
		fail("%v", err)
	}
	argumentMode := len(parsed.Chains) > 0

	me := strings.TrimSpace(string(gh("api", "user", "--jq", ".login")))

	var openPRs []prJSON
	if err := json.Unmarshal(gh("pr", "list", "--state", "open", "--limit", "200", "--json", "url,headRefName,baseRefName"), &openPRs); err != nil {
		fail("cannot read open pull requests: %v", err)
	}

	chains := parsed.Chains
	if !argumentMode {
		var todo []issueJSON
		raw := gh("issue", "list", "--assignee", "@me", "--state", "open", "--limit", "100", "--json", "number")
		if err := json.Unmarshal(raw, &todo); err != nil {
			fail("cannot list issues: %v", err)
		}
		for _, i := range todo {
			chains = append(chains, []int{i.Number})
		}
	}

	tickets := map[int]Ticket{}
	var queue []int
	for _, chain := range chains {
		queue = append(queue, chain...)
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, ok := tickets[id]; ok {
			continue
		}

		t := fetchIssue(id, me, openPRs)
		tickets[id] = t
		for _, b := range t.Blockers {
			queue = append(queue, b.ID)
		}
	}

	out, err := json.MarshalIndent(MakePlan(tickets, chains, parsed.Excluded, argumentMode), "", "  ")
	if err != nil {
		fail("%v", err)
	}
	fmt.Println(string(out))
}
