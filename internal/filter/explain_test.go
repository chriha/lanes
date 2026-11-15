package filter

import (
	"reflect"
	"testing"
)

func TestSplit(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		op        string
		operands  []string
		wantErr   bool
	}{
		{"empty", ``, "", nil, false},
		{"single clause", `draft`, "", []string{"draft"}, false},
		{"single comparison", `author == me`, "", []string{"author == me"}, false},
		{"and", `a && b`, "&&", []string{"a", "b"}, false},
		{"and chain is flat", `a && b && c`, "&&", []string{"a", "b", "c"}, false},
		{"or", `a || b`, "||", []string{"a", "b"}, false},
		{"or chain is flat", `a || b || c`, "||", []string{"a", "b", "c"}, false},
		{"not", `!draft`, "!", []string{"draft"}, false},
		{"word operators", `a and b`, "&&", []string{"a", "b"}, false},
		{"word or", `a or b`, "||", []string{"a", "b"}, false},
		{"word not", `not a`, "!", []string{"a"}, false},
		{"nested keeps deeper operators whole", `(a || b) && !c`, "&&", []string{"a || b", "!c"}, false},
		{"not of a group", `!(a && b)`, "!", []string{"a && b"}, false},
		{"mixed precedence", `a && b || c`, "||", []string{"a && b", "c"}, false},
		{"calls and strings", `x == "q" && hasAny(l, "a")`, "&&", []string{`x == "q"`, `hasAny(l, "a")`}, false},
		{"redundant parens", `(a)`, "", []string{"a"}, false},
		{"syntax error", `a &&`, "", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			op, operands, err := Split(tc.src)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if op != tc.op || !reflect.DeepEqual(operands, tc.operands) {
				t.Errorf("Split(%q) = %q, %q; want %q, %q", tc.src, op, operands, tc.op, tc.operands)
			}
		})
	}
}

func TestFailed(t *testing.T) {
	env := map[string]any{"author": "ann", "me": "bob", "labels": []string{"x"}, "draft": true}
	for _, tc := range []struct {
		name, src string
		want      []string
		wantErr   bool
	}{
		{"empty matches", ``, nil, false},
		{"match", `author == "ann"`, nil, false},
		{"and match", `author == "ann" && draft`, nil, false},
		{"single clause fails", `author == me`, []string{"author == me"}, false},
		{"and names only the failing operands", `author == me && draft && hasAny(labels, "y")`,
			[]string{"author == me", `hasAny(labels, "y")`}, false},
		{"or names all operands", `author == me || !draft`, []string{"author == me", "!draft"}, false},
		{"not names its operand", `!draft`, []string{"draft"}, false},
		{"nested stays whole", `(author == me || !draft) && draft`, []string{"author == me || !draft"}, false},
		{"syntax error", `a &&`, nil, true},
		{"operand run error", `author.x.y == 1`, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Failed(tc.src, env)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Failed(%q) = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
