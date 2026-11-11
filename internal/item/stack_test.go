package item

import (
	"reflect"
	"testing"
)

// mr is a merge request item with the given branch and target.
func mr(id, branch, target string) Item {
	return Item{ID: id, Fields: map[string]any{"branch": branch, "target": target}}
}

func TestStack(t *testing.T) {
	none := StackInfo{}
	tests := []struct {
		name  string
		items []Item
		want  []StackInfo
	}{
		{
			name:  "chain of three",
			items: []Item{mr("c", "c", "b"), mr("a", "a", "main"), mr("b", "b", "a")},
			want: []StackInfo{
				{Base: "b", Pos: 3, Size: 3, StackedBy: nil},
				{Base: "", Pos: 1, Size: 3, StackedBy: []string{"b"}},
				{Base: "a", Pos: 2, Size: 3, StackedBy: []string{"c"}},
			},
		},
		{
			name:  "two on one base",
			items: []Item{mr("a", "a", "main"), mr("b", "b", "a"), mr("c", "c", "a")},
			want: []StackInfo{
				{Pos: 1, Size: 3, StackedBy: []string{"b", "c"}},
				{Base: "a", Pos: 2, Size: 3},
				{Base: "a", Pos: 2, Size: 3},
			},
		},
		{
			name:  "target matches no branch",
			items: []Item{mr("a", "a", "main"), mr("b", "b", "gone")},
			want:  []StackInfo{none, none},
		},
		{
			name:  "empty target never stacks",
			items: []Item{mr("a", "", "main"), mr("b", "b", "")},
			want:  []StackInfo{none, none},
		},
		{
			name:  "empty branch is no base",
			items: []Item{mr("a", "", "main"), mr("b", "x", "")},
			want:  []StackInfo{none, none},
		},
		{
			name:  "branch equal to target is no base",
			items: []Item{mr("fork", "main", "main"), mr("b", "b", "main")},
			want:  []StackInfo{none, none},
		},
		{
			name:  "items without branch fields",
			items: []Item{{ID: "t1"}, {ID: "t2", Fields: map[string]any{"branch": 3, "target": 3}}},
			want:  []StackInfo{none, none},
		},
		{
			name:  "cycle is no stack",
			items: []Item{mr("a", "a", "b"), mr("b", "b", "a")},
			want:  []StackInfo{none, none},
		},
		{
			name:  "item on a cycle stacks on its base only",
			items: []Item{mr("a", "a", "b"), mr("b", "b", "a"), mr("c", "c", "a")},
			want: []StackInfo{
				{Pos: 1, Size: 2, StackedBy: []string{"c"}},
				none,
				{Base: "a", Pos: 2, Size: 2},
			},
		},
		{
			name:  "two separate stacks",
			items: []Item{mr("a", "a", "main"), mr("b", "b", "a"), mr("x", "x", "main"), mr("y", "y", "x"), mr("z", "z", "y")},
			want: []StackInfo{
				{Pos: 1, Size: 2, StackedBy: []string{"b"}},
				{Base: "a", Pos: 2, Size: 2},
				{Pos: 1, Size: 3, StackedBy: []string{"y"}},
				{Base: "x", Pos: 2, Size: 3, StackedBy: []string{"z"}},
				{Base: "y", Pos: 3, Size: 3},
			},
		},
		{name: "no items", items: nil, want: []StackInfo{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Stack(tt.items); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Stack = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestEnvStackDefaults(t *testing.T) {
	tests := []struct {
		name   string
		fields map[string]any
		key    string
		want   any
	}{
		{"base defaults to empty", nil, "base", ""},
		{"stack_pos defaults to 0", nil, "stack_pos", 0},
		{"stack_size defaults to 0", nil, "stack_size", 0},
		{"stacked_by defaults to empty list", nil, "stacked_by", []string{}},
		{"injected base", map[string]any{"base": "!4"}, "base", "!4"},
		{"provider stack_pos wins", map[string]any{"stack_pos": "top"}, "stack_pos", "top"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Item{Fields: tt.fields}).Env()[tt.key]; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("%s = %#v, want %#v", tt.key, got, tt.want)
			}
		})
	}
}
