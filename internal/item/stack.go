package item

// StackInfo is where an item sits in a stack of merge/pull requests, each targeting the previous one's branch.
type StackInfo struct {
	Base      string   // id of the item this one is stacked on; "" when none
	Pos       int      // 1 for the bottom of a stack, 2 for an item on it, …; 0 when in no stack
	Size      int      // number of items in the stack; 0 when in no stack
	StackedBy []string // ids of the items stacked directly on this one
}

// Stack returns the stack position of each item, in the order of items.
func Stack(items []Item) []StackInfo {
	return make([]StackInfo, len(items))
}
