package item

// SortKey is one field of a view's sort order.
type SortKey struct {
	Field string
	Desc  bool
}

// ParseSort parses a comma-separated list of field names; a "-" prefix sorts descending.
func ParseSort(spec string) ([]SortKey, error) { return nil, nil }

// Sort orders items by keys, stable; items missing a field sort last for that key.
func Sort(items []Item, keys []SortKey) {}

// compare orders two field values: numbers, bools and times by value, everything else as strings.
func compare(a, b any) int { return 0 }
