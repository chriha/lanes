package main

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Pure triage logic for /work-tickets: no network, no git.

const maxSlugLength = 60

// skipLabels keep a ticket away from the agents.
var skipLabels = []string{"on hold", "ready-for-human", "needs-info", "planning"}

type Blocker struct {
	ID       int    `json:"id"`
	Status   string `json:"status"`
	Resolved bool   `json:"resolved"`
}

type PR struct {
	URL    string `json:"url"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type Ticket struct {
	ID           int
	Title        string
	Labels       []string
	Priority     int // 1 urgent … 4 low, 0 = none
	CreatedAt    string
	AssignedToMe bool
	HasPlan      bool
	Blockers     []Blocker
	OpenPR       *PR
}

type Row struct {
	ID           int       `json:"id"`
	Title        string    `json:"title"`
	Priority     int       `json:"priority"`
	CreatedAt    string    `json:"createdAt"`
	Labels       []string  `json:"labels"`
	PlanApproved bool      `json:"planApproved"`
	Blockers     []Blocker `json:"blockers"`
	Class        string    `json:"class"`
	Reason       string    `json:"reason,omitempty"`
	Branch       string    `json:"branch,omitempty"`
	Base         string    `json:"base,omitempty"`
	Continues    *string   `json:"continues"`
	Model        string    `json:"model,omitempty"`
	Fix          *string   `json:"fix,omitempty"`
	FixReason    string    `json:"fixReason,omitempty"`
	Partial      string    `json:"partial,omitempty"`
}

type Skipped struct {
	ID     int    `json:"id"`
	Reason string `json:"reason"`
}

type Plan struct {
	Eligible []Row     `json:"eligible"`
	Chains   [][]int   `json:"chains"`
	Heads    []Row     `json:"heads"`
	Blocked  []Row     `json:"blocked"`
	Skipped  []Skipped `json:"skipped"`
}

type Argument struct {
	Chains   [][]int
	Excluded []int
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// ParseArgument reads `12>15 20 -37` into chains [[12 15] [20]] and exclusions [37].
func ParseArgument(argument string) (Argument, error) {
	var parsed Argument

	for _, token := range strings.Fields(argument) {
		if rest, ok := strings.CutPrefix(token, "-"); ok {
			id, err := parseID(rest)
			if err != nil {
				return parsed, err
			}
			if !slices.Contains(parsed.Excluded, id) {
				parsed.Excluded = append(parsed.Excluded, id)
			}

			continue
		}

		var chain []int
		for _, part := range strings.Split(token, ">") {
			id, err := parseID(part)
			if err != nil {
				return parsed, err
			}
			chain = append(chain, id)
		}
		parsed.Chains = append(parsed.Chains, chain)
	}

	return parsed, nil
}

func parseID(s string) (int, error) {
	var id int
	if _, err := fmt.Sscanf(strings.TrimPrefix(s, "#"), "%d", &id); err != nil || id < 1 {
		return 0, fmt.Errorf("not an issue number: %q", s)
	}

	return id, nil
}

// BranchName is `<type>/<n>-<slug>`, everything after `<type>/` at most 60 chars.
func BranchName(id int, title string, labels []string) string {
	kind := "chore"
	switch {
	case slices.Contains(labels, "bug"):
		kind = "fix"
	case slices.Contains(labels, "feature"):
		kind = "feature"
	case strings.HasPrefix(strings.ToLower(title), "refactor"):
		kind = "refactor"
	}

	slug := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if slug == "" {
		slug = "ticket"
	}
	name := fmt.Sprintf("%d-%s", id, slug)
	if len(name) > maxSlugLength {
		name = strings.TrimRight(name[:maxSlugLength], "-")
	}

	return kind + "/" + name
}

func planApproved(t Ticket) bool {
	return t.HasPlan && !slices.Contains(t.Labels, "planning")
}

func priorityRank(priority int) int {
	if priority == 0 {
		return 5
	}

	return priority
}

func row(t Ticket) Row {
	return Row{
		ID: t.ID, Title: t.Title, Priority: t.Priority, CreatedAt: t.CreatedAt, Labels: t.Labels,
		PlanApproved: planApproved(t), Blockers: []Blocker{},
	}
}

type verdict struct {
	class    string
	reason   string
	blockers []Blocker
}

func classify(t Ticket, earlierInChain []int) verdict {
	for _, label := range skipLabels {
		if slices.Contains(t.Labels, label) {
			return verdict{class: "skipped", reason: label}
		}
	}

	var blocking []Blocker
	for _, b := range t.Blockers {
		if !b.Resolved && !slices.Contains(earlierInChain, b.ID) {
			blocking = append(blocking, Blocker{ID: b.ID, Status: b.Status})
		}
	}
	if len(blocking) > 0 {
		described := make([]string, len(blocking))
		for i, b := range blocking {
			described[i] = fmt.Sprintf("#%d (%s)", b.ID, b.Status)
		}

		return verdict{class: "blocked", reason: "blocked by " + strings.Join(described, ", "), blockers: blocking}
	}

	return verdict{class: "eligible"}
}

// unresolved returns the open blockers of t, older first.
func unresolved(t Ticket, tickets map[int]Ticket) []int {
	var ids []int
	for _, b := range t.Blockers {
		if !b.Resolved {
			ids = append(ids, b.ID)
		}
	}
	sort.SliceStable(ids, func(i, j int) bool { return tickets[ids[i]].CreatedAt < tickets[ids[j]].CreatedAt })

	return ids
}

// buildChain puts blockers first (older first among independent ones) and the ticket last,
// with an open-PR blocker as the single head.
func buildChain(rootID int, tickets map[int]Ticket, excluded []int) (chain []int, errs []string) {
	var order, heads []int
	done := map[int]bool{}

	var visit func(id int, stack []int)
	visit = func(id int, stack []int) {
		if slices.Contains(stack, id) {
			errs = append(errs, fmt.Sprintf("cycle through #%d", id))

			return
		}
		if done[id] {
			return
		}
		done[id] = true

		t, ok := tickets[id]
		if !ok {
			errs = append(errs, fmt.Sprintf("#%d unknown", id))

			return
		}

		if id != rootID {
			if slices.Contains(excluded, id) {
				errs = append(errs, fmt.Sprintf("#%d excluded", id))

				return
			}
			if !t.AssignedToMe {
				errs = append(errs, fmt.Sprintf("#%d assigned to someone else", id))

				return
			}
			for _, label := range skipLabels {
				if slices.Contains(t.Labels, label) {
					errs = append(errs, fmt.Sprintf("#%d %s", id, label))

					return
				}
			}
			if t.OpenPR != nil {
				heads = append(heads, id)

				return
			}
		}

		for _, blockerID := range unresolved(t, tickets) {
			visit(blockerID, append(slices.Clone(stack), id))
		}
		order = append(order, id)
	}
	visit(rootID, nil)

	if len(heads) > 1 {
		errs = append(errs, "multiple open-PR blockers: "+joinIDs(heads))
	}

	return append(heads, order...), errs
}

func joinIDs(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("#%d", id)
	}

	return strings.Join(parts, ", ")
}

// suggestFix proposes a chain that works the blockers of id first.
func suggestFix(id int, tickets map[int]Ticket, excluded []int) (fix *string, fixReason, partial string) {
	chain, errs := buildChain(id, tickets, excluded)
	if len(errs) == 0 {
		s := joinChain(chain)

		return &s, "", ""
	}

	var workable []int
	heads := 0
	for _, blockerID := range unresolved(tickets[id], tickets) {
		attempt, attemptErrs := buildChain(blockerID, tickets, excluded)
		if len(attemptErrs) > 0 {
			continue
		}

		attemptHeads := 0
		for _, c := range attempt {
			if tickets[c].OpenPR != nil {
				attemptHeads++
			}
		}
		if heads+attemptHeads > 1 {
			continue
		}

		heads += attemptHeads
		for _, c := range attempt {
			if !slices.Contains(workable, c) {
				workable = append(workable, c)
			}
		}
	}

	if len(workable) > 0 {
		partial = joinChain(workable)
	}

	return nil, strings.Join(errs, "; "), partial
}

func joinChain(chain []int) string {
	parts := make([]string, len(chain))
	for i, id := range chain {
		parts[i] = fmt.Sprintf("#%d", id)
	}

	return strings.Join(parts, ">")
}

// MakePlan classifies the tickets of the given chains. Every id in chains and every blocker must be in tickets.
func MakePlan(tickets map[int]Ticket, chains [][]int, excluded []int, argumentMode bool) Plan {
	skipped := map[int]string{}
	var skippedOrder []int
	var kept [][]int
	seen := map[int]bool{}

	for _, chain := range chains {
		cut := 0
		for _, id := range chain {
			if cut == 0 && slices.Contains(excluded, id) {
				cut = id
				skipped[id] = "excluded by argument"
				skippedOrder = append(skippedOrder, id)

				continue
			}
			if cut != 0 {
				skipped[id] = fmt.Sprintf("excluded by argument (follows #%d in chain)", cut)
				skippedOrder = append(skippedOrder, id)
			}
		}

		var remaining []int
		for _, id := range chain {
			if _, ok := skipped[id]; ok {
				break
			}
			if !seen[id] {
				seen[id] = true
				remaining = append(remaining, id)
			}
		}
		if len(remaining) > 0 {
			kept = append(kept, remaining)
		}
	}

	rows := map[int]Row{}
	var rowOrder []int
	chainsOut := [][]int{}
	put := func(r Row) {
		rows[r.ID] = r
		rowOrder = append(rowOrder, r.ID)
	}

	for _, chain := range kept {
		previousBranch := ""
		stoppedAt := 0

		for position, id := range chain {
			t := tickets[id]
			r := row(t)

			if stoppedAt != 0 {
				r.Class = "blocked"
				r.Reason = fmt.Sprintf("not eligible: chain stopped at #%d", stoppedAt)
				put(r)

				continue
			}

			v := classify(t, chain[:position])

			if position == 0 && len(chain) > 1 && t.OpenPR != nil {
				r.Class = "head"
				r.Branch = t.OpenPR.Source
				put(r)
				previousBranch = t.OpenPR.Source

				continue
			}

			if v.class != "eligible" {
				stoppedAt = id
				r.Class = v.class
				r.Reason = v.reason
				if position > 0 {
					r.Class = "blocked"
					r.Reason = "not eligible: " + v.reason
				}
				if v.blockers != nil {
					r.Blockers = v.blockers
				}
				put(r)

				continue
			}

			branch := BranchName(id, t.Title, t.Labels)
			base := "main"
			if t.OpenPR != nil {
				branch, base = t.OpenPR.Source, t.OpenPR.Target
				r.Continues = &t.OpenPR.URL
			} else if previousBranch != "" {
				base = previousBranch
			}
			r.Class, r.Branch, r.Base, r.Model = "eligible", branch, base, "sonnet"
			if r.PlanApproved {
				r.Model = "opus"
			}
			put(r)
			previousBranch = branch
		}

		if len(chain) > 1 {
			chainsOut = append(chainsOut, chain)
		}
	}

	if argumentMode {
		for _, id := range rowOrder {
			r := rows[id]
			if r.Class == "blocked" && len(r.Blockers) > 0 {
				r.Fix, r.FixReason, r.Partial = suggestFix(id, tickets, excluded)
				rows[id] = r
			}
		}
	}

	pick := func(class string) []Row {
		picked := []Row{}
		for _, id := range rowOrder {
			if rows[id].Class == class {
				picked = append(picked, rows[id])
			}
		}

		return picked
	}

	eligible := pick("eligible")
	sort.SliceStable(eligible, func(i, j int) bool {
		a, b := eligible[i], eligible[j]
		if ra, rb := priorityRank(a.Priority), priorityRank(b.Priority); ra != rb {
			return ra < rb
		}

		return a.CreatedAt < b.CreatedAt
	})

	skippedOut := []Skipped{}
	for _, r := range pick("skipped") {
		skippedOut = append(skippedOut, Skipped{ID: r.ID, Reason: r.Reason})
	}
	for _, id := range skippedOrder {
		skippedOut = append(skippedOut, Skipped{ID: id, Reason: skipped[id]})
	}

	return Plan{Eligible: eligible, Chains: chainsOut, Heads: pick("head"), Blocked: pick("blocked"), Skipped: skippedOut}
}
