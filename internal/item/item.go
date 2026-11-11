// Package item defines the generic work item every provider produces.
package item

type Badge struct {
	Text string `json:"text"`
	Kind string `json:"kind"` // ok | warn | bad | info | dim
}

type Item struct {
	ID     string         `json:"id"`
	Title  string         `json:"title"`
	URL    string         `json:"url"`
	Author string         `json:"author"`
	Draft  bool           `json:"draft"`
	Badges []Badge        `json:"badges"`
	Fields map[string]any `json:"fields"`
	// Pending lists fields a partial (quick) result could not compute; they are inherited from the previous snapshot.
	Pending []string `json:"pending,omitempty"`
}

// Env is the variable set filter expressions are evaluated against.
func (i Item) Env() map[string]any {
	env := make(map[string]any, len(i.Fields)+12)
	for k, v := range i.Fields {
		env[k] = v
	}
	env["id"], env["title"], env["url"], env["author"], env["draft"] = i.ID, i.Title, i.URL, i.Author, i.Draft
	if _, ok := env["note"]; !ok {
		env["note"] = "" // notes are injected into Fields by the UI; a provider field named note wins
	}
	if _, ok := env["snoozed"]; !ok {
		env["snoozed"] = false // injected by the UI like note
	}
	if _, ok := env["unread"]; !ok {
		env["unread"] = false // injected by the UI like note
	}
	// the stack fields (see Stack) are injected by the UI like note; these are the values of an item in no stack
	if _, ok := env["base"]; !ok {
		env["base"] = ""
	}
	if _, ok := env["stack_pos"]; !ok {
		env["stack_pos"] = 0
	}
	if _, ok := env["stack_size"]; !ok {
		env["stack_size"] = 0
	}
	if _, ok := env["stacked_by"]; !ok {
		env["stacked_by"] = []string{}
	}
	return env
}
