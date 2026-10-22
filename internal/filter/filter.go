// Package filter compiles view filter expressions (expr-lang) against item fields.
package filter

import (
	"fmt"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

type Filter struct{ prog *vm.Program }

// Compile accepts an empty expression, which matches everything.
func Compile(src string) (*Filter, error) {
	if src == "" {
		return &Filter{}, nil
	}
	p, err := expr.Compile(src, expr.AsBool(), expr.AllowUndefinedVariables())
	if err != nil {
		return nil, err
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
