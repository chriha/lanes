package filter

import (
	"strings"
	"testing"
)

func TestMatch(t *testing.T) {
	env := map[string]any{"author": "ann", "me": "bob", "labels": []string{"x"}, "draft": false}
	for src, want := range map[string]bool{
		``:                              true,
		`author != me && !draft`:        true,
		`"x" in labels`:                 true,
		`"y" in labels || author == me`: false,
	} {
		f, err := Compile(src)
		if err != nil {
			t.Fatal(src, err)
		}
		if got, err := f.Match(env); err != nil || got != want {
			t.Errorf("%q: got %v, %v want %v", src, got, err, want)
		}
	}
	if _, err := Compile(`nonsense((`); err == nil {
		t.Error("bad expression should fail to compile")
	}
}

func TestListHelpers(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		env       map[string]any
		want      bool
	}{
		{"any match", `hasAny(labels, "a", "z")`, map[string]any{"labels": []any{"a", "b"}}, true},
		{"any no match", `hasAny(labels, "y", "z")`, map[string]any{"labels": []string{"a"}}, false},
		{"any nil", `hasAny(labels, "a")`, map[string]any{"labels": nil}, false},
		{"any missing", `hasAny(labels, "a")`, map[string]any{}, false},
		{"any no values", `hasAny(labels)`, map[string]any{"labels": []any{"a"}}, false},
		{"none match", `hasNone(labels, "a", "z")`, map[string]any{"labels": []any{"a", "b"}}, false},
		{"none no match", `hasNone(labels, "y", "z")`, map[string]any{"labels": []string{"a"}}, true},
		{"none nil", `hasNone(labels, "a")`, map[string]any{"labels": nil}, true},
		{"none missing", `hasNone(labels, "a")`, map[string]any{}, true},
		{"none no values", `hasNone(labels)`, map[string]any{"labels": []any{"a"}}, true},
		{"all match", `hasAll(labels, "a", "b")`, map[string]any{"labels": []any{"a", "b", "c"}}, true},
		{"all no match", `hasAll(labels, "a", "z")`, map[string]any{"labels": []string{"a", "b"}}, false},
		{"all nil", `hasAll(labels, "a")`, map[string]any{"labels": nil}, false},
		{"all missing", `hasAll(labels, "a")`, map[string]any{}, false},
		{"all no values", `hasAll(labels)`, map[string]any{}, true},
		{"negated", `!hasAny(labels, "a")`, map[string]any{"labels": []string{"b"}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Compile(tc.src)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := f.Match(tc.env); err != nil || got != tc.want {
				t.Errorf("got %v, %v want %v", got, err, tc.want)
			}
		})
	}
}

func TestListLiteralInRejected(t *testing.T) {
	for _, src := range []string{
		`["a", "b"] in labels`,
		`["a", "b"] not in labels`,
		`!draft && ["a"] not in labels`,
	} {
		_, err := Compile(src)
		if err == nil || !strings.Contains(err.Error(), "hasAny") || !strings.Contains(err.Error(), "hasNone") {
			t.Errorf("%q: want error naming hasAny/hasNone, got %v", src, err)
		}
	}
	for _, src := range []string{`"x" in labels`, `"x" not in labels`, `author in ["a", "b"]`} {
		if _, err := Compile(src); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}
