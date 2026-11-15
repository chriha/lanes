package filter

import (
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// Split cuts src at its top-level `&&`, `||` or `!` and returns that operator ("" when src has none) and the
// operands' source. A chain of one operator is flat (`a && b && c` has three operands); deeper operators stay
// inside their operand. The operands are printed from the parsed expression, so spacing and parentheses may differ
// from src. An empty src has no operands.
func Split(src string) (op string, operands []string, err error) {
	if src == "" {
		return "", nil, nil
	}
	tree, err := parser.Parse(src)
	if err != nil {
		return "", nil, err
	}
	switch n := tree.Node.(type) {
	case *ast.BinaryNode:
		if op := canonicalOp(n.Operator); op == "&&" || op == "||" {
			return op, flatten(n, op, nil), nil
		}
	case *ast.UnaryNode:
		if canonicalOp(n.Operator) == "!" {
			return "!", []string{n.Node.String()}, nil
		}
	}
	return "", []string{tree.Node.String()}, nil
}

// canonicalOp maps the word forms (and, or, not) to the symbols.
func canonicalOp(op string) string {
	switch op {
	case "and":
		return "&&"
	case "or":
		return "||"
	case "not":
		return "!"
	}
	return op
}

func flatten(n ast.Node, op string, out []string) []string {
	if b, ok := n.(*ast.BinaryNode); ok && canonicalOp(b.Operator) == op {
		return flatten(b.Right, op, flatten(b.Left, op, out))
	}
	return append(out, n.String())
}

// Failed returns the top-level operands that made src not match env: for `&&` the operands that are false, for
// `||` all of them, for `!` its operand, otherwise the whole expression. It is nil when src matches.
func Failed(src string, env map[string]any) ([]string, error) {
	f, err := Compile(src)
	if err != nil {
		return nil, err
	}
	ok, err := f.Match(env)
	if err != nil || ok {
		return nil, err
	}
	op, operands, err := Split(src)
	if err != nil {
		return nil, err
	}
	if op != "&&" {
		return operands, nil // `||` and `!`: every operand is to blame; a single clause is the whole expression
	}
	var failed []string
	for _, o := range operands {
		of, err := Compile(o)
		if err != nil {
			return nil, err
		}
		if ok, err := of.Match(env); err != nil {
			return nil, err
		} else if !ok {
			failed = append(failed, o)
		}
	}
	return failed, nil
}
