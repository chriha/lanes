package provider

// relLinks collects a ticket's relations to other tickets: the open ids per relation, and the closed ones as
// "<relation>:<id>" (the `closed_links` field).
type relLinks struct {
	blockedBy, blocking, related, closed []string
}

func newRelLinks() *relLinks {
	return &relLinks{blockedBy: []string{}, blocking: []string{}, related: []string{}, closed: []string{}}
}

// add records id under relation rel ("blocked_by", "blocking" or "related").
func (l *relLinks) add(rel, id string, open bool) {
	if !open {
		l.closed = append(l.closed, rel+":"+id)
		return
	}
	switch rel {
	case "blocked_by":
		l.blockedBy = append(l.blockedBy, id)
	case "blocking":
		l.blocking = append(l.blocking, id)
	case "related":
		l.related = append(l.related, id)
	}
}

// fields adds the link fields to a ticket's field map.
func (l *relLinks) fields(f map[string]any) map[string]any {
	f["blocked_by"], f["blocking"], f["related"], f["closed_links"] = l.blockedBy, l.blocking, l.related, l.closed
	return f
}
