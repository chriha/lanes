package ui

import (
	"maps"

	"github.com/chriha/lanes/internal/item"
)

// withStack returns the item with its stack fields (base, stack_pos, stack_size, stacked_by) in a copy of its fields,
// when it is in a stack (item.Env defaults them for an item in none). A provider field of the same name wins, as
// with note.
func withStack(it item.Item, s item.StackInfo) item.Item {
	if s.Pos == 0 {
		return it
	}
	f := make(map[string]any, len(it.Fields)+4)
	f["base"], f["stack_pos"], f["stack_size"], f["stacked_by"] = s.Base, s.Pos, s.Size, stackedBy(s)
	maps.Copy(f, it.Fields) // the provider's fields win
	it.Fields = f
	return it
}

// stackedBy is s.StackedBy, never nil, so filters see an empty list rather than nil.
func stackedBy(s item.StackInfo) []string {
	if s.StackedBy == nil {
		return []string{}
	}
	return s.StackedBy
}

// stackBase is the id of the item it is stacked on (its base field), "" when none.
func stackBase(it item.Item) string {
	b, _ := it.Fields["base"].(string)
	return b
}
