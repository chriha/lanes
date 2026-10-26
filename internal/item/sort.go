package item

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"
)

// SortKey is one field of a view's sort order.
type SortKey struct {
	Field string
	Desc  bool
}

// ParseSort parses a comma-separated list of field names; a "-" prefix sorts descending.
// Field names are not checked (fields are provider-defined), but empty keys are an error.
func ParseSort(spec string) ([]SortKey, error) {
	if spec == "" {
		return nil, nil
	}
	var keys []SortKey
	for _, part := range strings.Split(spec, ",") {
		k := SortKey{Field: strings.TrimSpace(part)}
		if f, ok := strings.CutPrefix(k.Field, "-"); ok {
			k.Field, k.Desc = strings.TrimSpace(f), true
		}
		if k.Field == "" {
			return nil, fmt.Errorf("sort %q: empty key", spec)
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// Sort orders items by keys, stable; items missing a field sort last for that key.
func Sort(items []Item, keys []SortKey) {
	if len(keys) == 0 {
		return
	}
	envs := make(map[string]map[string]any, len(items))
	for _, it := range items {
		envs[it.ID] = it.Env()
	}
	slices.SortStableFunc(items, func(a, b Item) int {
		for _, k := range keys {
			va, vb := envs[a.ID][k.Field], envs[b.ID][k.Field]
			switch {
			case va == nil && vb == nil:
				continue
			case va == nil:
				return 1
			case vb == nil:
				return -1
			}
			c := compare(va, vb)
			if k.Desc {
				c = -c
			}
			if c != 0 {
				return c
			}
		}
		return 0
	})
}

// compare orders two field values: numbers, bools and times by value, everything else as strings.
func compare(a, b any) int {
	if x, ok := number(a); ok {
		if y, ok := number(b); ok {
			return cmp.Compare(x, y)
		}
	}
	if x, ok := a.(bool); ok {
		if y, ok := b.(bool); ok {
			switch {
			case x == y:
				return 0
			case y:
				return -1
			}
			return 1
		}
	}
	if x, ok := a.(time.Time); ok {
		if y, ok := b.(time.Time); ok {
			return x.Compare(y)
		}
	}
	return strings.Compare(fmt.Sprint(a), fmt.Sprint(b))
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}
