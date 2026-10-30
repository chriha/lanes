package provider

import "slices"

// prFields are the keys of a merge/pull request item (toItem); gitlab adds iid, github number.
var prFields = []string{"approved", "approved_by", "approved_by_me", "assignees", "branch", "ci", "conflicts", "diverged",
	"kind", "labels", "me", "open_thread_by_me", "open_threads", "reviewers", "target", "ticket"}

// ticketFields are the keys of a ticket item; github-issues adds number, linear key.
var ticketFields = []string{"assignees", "blocked", "blocked_by", "blocking", "closed_links", "kind", "labels", "me", "priority", "related", "status"}

// Fields returns the sorted keys of the fields the items of a built-in provider type carry (the keys filters and
// action templates can use besides id, title, url, author, draft and note). It returns nil for "command", whose
// fields come from the command's output, and for unknown types.
func Fields(typ string) []string {
	var keys []string
	switch typ {
	case "gitlab":
		keys = append(slices.Clone(prFields), "iid")
	case "github":
		keys = append(slices.Clone(prFields), "number")
	case "github-issues":
		keys = append(slices.Clone(ticketFields), "number")
	case "linear":
		keys = append(slices.Clone(ticketFields), "key")
	default:
		return nil
	}
	slices.Sort(keys)
	return keys
}
