package item

import (
	"reflect"
	"testing"
	"time"
)

func TestParseSort(t *testing.T) {
	tests := []struct {
		spec    string
		want    []SortKey
		wantErr bool
	}{
		{"", nil, false},
		{"title", []SortKey{{"title", false}}, false},
		{"-updated_at, title", []SortKey{{"updated_at", true}, {"title", false}}, false},
		{"a,,b", nil, true},
		{"-", nil, true},
		{"a,", nil, true},
		{" ", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			got, err := ParseSort(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCompare(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		a, b any
		want int
	}{
		{"numbers by value, not text", 9.0, 10.0, -1},
		{"mixed number types", 3, 2.5, 1},
		{"equal numbers", int64(4), 4, 0},
		{"false before true", false, true, -1},
		{"equal bools", true, true, 0},
		{"times ordered", t1.Add(time.Hour), t1, 1},
		{"strings", "apple", "banana", -1},
		{"mixed kinds as strings", "10", 9.0, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compare(tt.a, tt.b); got != tt.want {
				t.Errorf("compare(%v, %v) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestSort(t *testing.T) {
	it := func(id string, f map[string]any) Item { return Item{ID: id, Title: "t" + id, Fields: f} }
	items := []Item{
		it("1", map[string]any{"n": 2.0, "ci": "ok"}),
		it("2", map[string]any{"n": 1.0, "ci": "failed"}),
		it("3", map[string]any{"ci": "ok"}),
		it("4", map[string]any{"n": 2.0, "ci": "failed"}),
	}
	tests := []struct {
		name string
		spec string
		want []string
	}{
		{"no keys keeps provider order", "", []string{"1", "2", "3", "4"}},
		{"ascending, stable, missing last", "n", []string{"2", "1", "4", "3"}},
		{"descending, missing still last", "-n", []string{"1", "4", "2", "3"}},
		{"second key breaks ties", "-n,ci", []string{"4", "1", "2", "3"}},
		{"built-in fields", "-id", []string{"4", "3", "2", "1"}},
		{"unknown field keeps order", "nope", []string{"1", "2", "3", "4"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys, err := ParseSort(tt.spec)
			if err != nil {
				t.Fatal(err)
			}
			got := append([]Item(nil), items...)
			Sort(got, keys)
			ids := make([]string, len(got))
			for i, g := range got {
				ids[i] = g.ID
			}
			if !reflect.DeepEqual(ids, tt.want) {
				t.Errorf("got %v, want %v", ids, tt.want)
			}
		})
	}
}
