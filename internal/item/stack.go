package item

// StackInfo is where an item sits in a stack of merge/pull requests, each targeting the previous one's branch.
type StackInfo struct {
	Base      string   // id of the item this one is stacked on; "" when none
	Pos       int      // 1 for the bottom of a stack, 2 for an item on it, …; 0 when in no stack
	Size      int      // number of items in the stack; 0 when in no stack
	StackedBy []string // ids of the items stacked directly on this one
}

// Stack returns the stack position of each item, in the order of items. Item B is stacked on item A when B's
// `target` field equals A's `branch` field, both strings and non-empty, and A's branch is not its own target (a fork
// PR from its main into main is nobody's base). When several items share a branch, the first one is the base. Items
// whose bases form a cycle get no base, so the items on them start a stack of their own.
func Stack(items []Item) []StackInfo {
	out := make([]StackInfo, len(items))
	owner := make(map[string]int) // branch -> index of the first item that can be a base
	for i, it := range items {
		b, t := it.str("branch"), it.str("target")
		if b == "" || t == "" || b == t {
			continue
		}
		if _, ok := owner[b]; !ok {
			owner[b] = i
		}
	}
	base := make([]int, len(items)) // index of the base, -1 for none
	for i, it := range items {
		base[i] = -1
		if it.str("branch") == "" {
			continue
		}
		if j, ok := owner[it.str("target")]; ok && j != i {
			base[i] = j
		}
	}
	dropCycles(base)

	pos := make([]int, len(items))
	root := make([]int, len(items))
	var place func(i int)
	place = func(i int) {
		if pos[i] != 0 {
			return
		}
		if base[i] < 0 {
			pos[i], root[i] = 1, i
			return
		}
		place(base[i])
		pos[i], root[i] = pos[base[i]]+1, root[base[i]]
	}
	size := make([]int, len(items))
	for i := range items {
		place(i)
		if j := base[i]; j >= 0 {
			out[j].StackedBy = append(out[j].StackedBy, items[i].ID)
			out[i].Base = items[j].ID
		}
	}
	for i := range items {
		if base[i] >= 0 || out[i].StackedBy != nil {
			size[root[i]]++
		}
	}
	for i := range items {
		if base[i] >= 0 || out[i].StackedBy != nil {
			out[i].Pos, out[i].Size = pos[i], size[root[i]]
		}
	}
	return out
}

// dropCycles clears the base of every item on a cycle of bases, so following bases always ends.
func dropCycles(base []int) {
	const (
		unvisited = iota
		onPath
		done
	)
	state := make([]int, len(base))
	for start := range base {
		var path []int
		i := start
		for i >= 0 && state[i] == unvisited {
			state[i] = onPath
			path = append(path, i)
			i = base[i]
		}
		if i >= 0 && state[i] == onPath { // the walk came back to i: path from i on is a cycle
			for k := len(path) - 1; path[k] != i; k-- {
				base[path[k]] = -1
			}
			base[i] = -1
		}
		for _, k := range path {
			state[k] = done
		}
	}
}

// str returns the string field key, or "" when it is missing or not a string.
func (i Item) str(key string) string {
	s, _ := i.Fields[key].(string)
	return s
}
