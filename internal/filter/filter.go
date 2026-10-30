// Package filter compiles view filter expressions (expr-lang) against item fields.
package filter

import (
	"fmt"
	"reflect"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/vm"
)

type Filter struct{ prog *vm.Program }

// Compile accepts an empty expression, which matches everything.
func Compile(src string) (*Filter, error) {
	if src == "" {
		return &Filter{}, nil
	}
	v := &listIn{}
	p, err := expr.Compile(src, expr.AsBool(), expr.Patch(v), expr.AllowUndefinedVariables(),
		helper("hasAny", func(n, vals int) bool { return n > 0 }),
		helper("hasNone", func(n, vals int) bool { return n == 0 }),
		helper("hasAll", func(n, vals int) bool { return n == vals }))
	if err != nil {
		return nil, err
	}
	if v.found {
		return nil, fmt.Errorf("a list on the left of `in` always compares the whole list to one element; use hasAny, hasNone or hasAll, e.g. hasNone(labels, \"a\", \"b\")")
	}
	return &Filter{prog: p}, nil
}

func (f *Filter) Match(env map[string]any) (bool, error) {
	if f.prog == nil {
		return true, nil
	}
	out, err := expr.Run(f.prog, env)
	if err != nil {
		return false, err
	}
	b, ok := out.(bool)
	if !ok {
		return false, fmt.Errorf("filter returned %T, want bool", out)
	}
	return b, nil
}

// helper builds hasAny/hasNone/hasAll(list, values...): ok gets how many of the values are in the list and
// how many were given. A missing or nil list counts as empty.
func helper(name string, ok func(n, vals int) bool) expr.Option {
	return expr.Function(name, func(args ...any) (any, error) {
		list := reflect.ValueOf(args[0])
		vals := args[1:]
		n := 0
		for _, v := range vals {
			if contains(list, v) {
				n++
			}
		}
		return ok(n, len(vals)), nil
	}, new(func(any, ...any) bool))
}

func contains(list reflect.Value, v any) bool {
	if !list.IsValid() || (list.Kind() != reflect.Slice && list.Kind() != reflect.Array) {
		return false
	}
	for i := 0; i < list.Len(); i++ {
		if list.Index(i).Interface() == v {
			return true
		}
	}
	return false
}

// listIn flags `[...] in x` / `[...] not in x`.
type listIn struct{ found bool }

func (l *listIn) Visit(n *ast.Node) {
	if b, ok := (*n).(*ast.BinaryNode); ok && b.Operator == "in" {
		if _, isList := b.Left.(*ast.ArrayNode); isList {
			l.found = true
		}
	}
}
