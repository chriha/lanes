package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseArgument(t *testing.T) {
	got, err := ParseArgument("#12>15 20 -37 -37")
	if err != nil {
		t.Fatal(err)
	}

	want := Argument{Chains: [][]int{{12, 15}, {20}}, Excluded: []int{37}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseArgumentRejectsNonNumbers(t *testing.T) {
	if _, err := ParseArgument("EQD-12"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestBranchName(t *testing.T) {
	cases := []struct {
		title  string
		labels []string
		want   string
	}{
		{"Keep null placings out", []string{"bug"}, "fix/12-keep-null-placings-out"},
		{"Add a --json flag", []string{"feature"}, "feature/12-add-a-json-flag"},
		{"Refactor the cache", nil, "refactor/12-refactor-the-cache"},
		{"???", nil, "chore/12-ticket"},
	}
	for _, c := range cases {
		if got := BranchName(12, c.title, c.labels); got != c.want {
			t.Errorf("BranchName(%q) = %q, want %q", c.title, got, c.want)
		}
	}
}

func TestBranchNameCutsTheSlugAtSixtyChars(t *testing.T) {
	got := BranchName(12, strings.Repeat("word ", 30), nil)

	name := strings.TrimPrefix(got, "chore/")
	if len(name) > maxSlugLength || strings.HasSuffix(name, "-") {
		t.Fatalf("slug %q too long or ends in a dash", name)
	}
}

func TestEligibleTicketsAreOrderedByPriorityThenAge(t *testing.T) {
	tickets := map[int]Ticket{
		1: {ID: 1, Title: "old, no priority", CreatedAt: "2026-01-01", AssignedToMe: true},
		2: {ID: 2, Title: "newer, urgent", CreatedAt: "2026-02-01", Priority: 1, AssignedToMe: true},
		3: {ID: 3, Title: "newest, urgent", CreatedAt: "2026-03-01", Priority: 1, AssignedToMe: true},
	}

	plan := MakePlan(tickets, [][]int{{1}, {2}, {3}}, nil, false)

	var ids []int
	for _, r := range plan.Eligible {
		ids = append(ids, r.ID)
	}
	if !reflect.DeepEqual(ids, []int{2, 3, 1}) {
		t.Fatalf("got order %v", ids)
	}
}

func TestSkipLabelsAndOpenBlockers(t *testing.T) {
	tickets := map[int]Ticket{
		1: {ID: 1, Labels: []string{"needs-info"}},
		2: {ID: 2, Blockers: []Blocker{{ID: 3, Status: "open"}}},
		3: {ID: 3},
		4: {ID: 4, Blockers: []Blocker{{ID: 3, Status: "closed", Resolved: true}}},
	}

	plan := MakePlan(tickets, [][]int{{1}, {2}, {4}}, nil, false)

	if len(plan.Skipped) != 1 || plan.Skipped[0].Reason != "needs-info" {
		t.Errorf("skipped: %+v", plan.Skipped)
	}
	if len(plan.Blocked) != 1 || plan.Blocked[0].ID != 2 || plan.Blocked[0].Reason != "blocked by #3 (open)" {
		t.Errorf("blocked: %+v", plan.Blocked)
	}
	if len(plan.Eligible) != 1 || plan.Eligible[0].ID != 4 {
		t.Errorf("eligible: %+v", plan.Eligible)
	}
}

func TestChainStacksOnThePreviousBranch(t *testing.T) {
	tickets := map[int]Ticket{
		1: {ID: 1, Title: "first"},
		2: {ID: 2, Title: "second", Blockers: []Blocker{{ID: 1, Status: "open"}}},
	}

	plan := MakePlan(tickets, [][]int{{1, 2}}, nil, true)

	if len(plan.Eligible) != 2 {
		t.Fatalf("eligible: %+v", plan.Eligible)
	}
	byID := map[int]Row{plan.Eligible[0].ID: plan.Eligible[0], plan.Eligible[1].ID: plan.Eligible[1]}
	if byID[1].Base != "main" || byID[2].Base != byID[1].Branch {
		t.Errorf("bases: #1 %q, #2 %q (branch of #1: %q)", byID[1].Base, byID[2].Base, byID[1].Branch)
	}
	if !reflect.DeepEqual(plan.Chains, [][]int{{1, 2}}) {
		t.Errorf("chains: %v", plan.Chains)
	}
}

func TestOpenPRHeadsAChainAndIsNotWorked(t *testing.T) {
	tickets := map[int]Ticket{
		1: {ID: 1, OpenPR: &PR{URL: "u", Source: "fix/1-a", Target: "main"}},
		2: {ID: 2, Blockers: []Blocker{{ID: 1, Status: "open"}}},
	}

	plan := MakePlan(tickets, [][]int{{1, 2}}, nil, true)

	if len(plan.Heads) != 1 || plan.Heads[0].ID != 1 {
		t.Fatalf("heads: %+v", plan.Heads)
	}
	if len(plan.Eligible) != 1 || plan.Eligible[0].Base != "fix/1-a" {
		t.Fatalf("eligible: %+v", plan.Eligible)
	}
}

func TestExcludingATicketDropsTheRestOfItsChain(t *testing.T) {
	tickets := map[int]Ticket{1: {ID: 1}, 2: {ID: 2}, 3: {ID: 3}}

	plan := MakePlan(tickets, [][]int{{1, 2, 3}}, []int{2}, true)

	if len(plan.Eligible) != 1 || plan.Eligible[0].ID != 1 {
		t.Errorf("eligible: %+v", plan.Eligible)
	}
	if len(plan.Skipped) != 2 {
		t.Errorf("skipped: %+v", plan.Skipped)
	}
}

func TestBlockedArgumentTicketGetsAFixChain(t *testing.T) {
	tickets := map[int]Ticket{
		1: {ID: 1, AssignedToMe: true},
		2: {ID: 2, AssignedToMe: true, Blockers: []Blocker{{ID: 1, Status: "open"}}},
	}

	plan := MakePlan(tickets, [][]int{{2}}, nil, true)

	if len(plan.Blocked) != 1 || plan.Blocked[0].Fix == nil || *plan.Blocked[0].Fix != "#1>#2" {
		t.Fatalf("blocked: %+v", plan.Blocked)
	}
}

func TestFixIsRefusedWhenABlockerIsOnHold(t *testing.T) {
	tickets := map[int]Ticket{
		1: {ID: 1, AssignedToMe: true, Labels: []string{"on hold"}},
		2: {ID: 2, AssignedToMe: true, Blockers: []Blocker{{ID: 1, Status: "open"}}},
	}

	plan := MakePlan(tickets, [][]int{{2}}, nil, true)

	blocked := plan.Blocked[0]
	if blocked.Fix != nil || !strings.Contains(blocked.FixReason, "#1 on hold") {
		t.Fatalf("blocked: %+v", blocked)
	}
}
