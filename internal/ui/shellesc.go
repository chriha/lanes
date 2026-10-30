package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"text/template"
	"text/template/parse"
)

// An action's run template is shell code and item fields are untrusted (a branch can be called
// x$(curl…|sh)), so every value a run template inserts is escaped for the shell quote context it lands in,
// like html/template does for HTML: single-quoted outside quotes, backslash-escaped inside "…".
// After parsing, escapeShell walks the template's text to track that context and appends an escaper to
// each {{…}}. A context it cannot tell for sure is an error, never a guess. {{.x | raw}} opts out;
// {{.x | q}} is the same as {{.x}}.

const (
	fnShq = "_lanes_shq" // escaper outside quotes
	fnDq  = "_lanes_dq"  // escaper inside "…"
)

var templateFuncs = template.FuncMap{
	"q":   shqValue,
	"raw": func(v any) any { return v },
	fnShq: shqValue,
	fnDq:  dqValue,
}

func valueString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func shqValue(v any) string { return shq(valueString(v)) }

var dqEscaper = strings.NewReplacer(`\`, `\\`, `$`, `\$`, "`", "\\`", `"`, `\"`)

func dqValue(v any) string { return dqEscaper.Replace(valueString(v)) }

type quoting uint8

const (
	qOut   quoting = iota // unquoted: the top level, or inside $(…)
	qDq                   // "…"
	qSq                   // '…'
	qAnsi                 // $'…'
	qTick                 // `…`
	qBrace                // ${…}
)

type frame struct {
	q     quoting
	depth int // open ( inside a $(…) frame
}

// prev classes: what the byte before the current position was, as far as # comments, case and $ care.
const (
	prevOther     = iota
	prevWordStart // start of text, blank or an operator: a # here starts a comment
	prevDollar
	prevEither // differs between template branches: word start or not
)

// shellState is the shell quote context at a point of the template's text.
type shellState struct {
	stack   []frame // innermost last; never empty
	esc     bool    // a backslash escapes the next byte
	comment bool    // inside a # comment, until the newline
	prev    int
	unknown string // why the context cannot be told; any placeholder from here on is an error
}

func (s shellState) equal(o shellState) bool {
	return slices.Equal(s.stack, o.stack) && s.esc == o.esc && s.comment == o.comment && s.unknown == o.unknown
}

func (s *shellState) push(q quoting) { s.stack = append(slices.Clip(s.stack), frame{q: q}) }
func (s *shellState) pop()           { s.stack = s.stack[:len(s.stack)-1] }

func isBlank(c byte) bool { return c == ' ' || c == '\t' || c == '\n' }

func wordStartAfter(c byte) bool { return isBlank(c) || strings.IndexByte(";&|()<>", c) >= 0 }

// scan advances s over literal shell text.
func (s shellState) scan(text string) shellState {
	for i := 0; i < len(text) && s.unknown == ""; i++ {
		c := text[i]
		next := byte(0)
		if i+1 < len(text) {
			next = text[i+1]
		}
		if s.comment {
			s.comment = c != '\n'
			s.prev = prevWordStart
			continue
		}
		if s.esc {
			s.esc, s.prev = false, prevOther
			continue
		}
		top := &s.stack[len(s.stack)-1]
		prev := s.prev
		s.prev = prevOther
		switch top.q {
		case qSq:
			if c == '\'' {
				s.pop()
			}
		case qAnsi:
			if c == '\\' {
				s.esc = true
			} else if c == '\'' {
				s.pop()
			}
		case qTick:
			switch c {
			case '\\':
				s.esc = true
			case '`':
				s.pop()
			case '\'', '"':
				s.unknown = "quotes inside `…`; use $(…)"
			}
		case qBrace:
			switch {
			case c == '\\':
				s.esc = true
			case c == '}':
				s.pop()
			case c == '\'' || c == '"' || c == '`' || c == '$' && (next == '(' || next == '{'):
				s.unknown = "quotes or expansions inside ${…}"
			}
		case qDq:
			switch {
			case c == '\\':
				s.esc = true
			case c == '"':
				s.pop()
			case c == '`':
				s.push(qTick)
			case c == '$' && next == '(':
				s.push(qOut)
				s.prev = prevWordStart
				i++
			case c == '$' && next == '{':
				s.push(qBrace)
				i++
			case c == '$':
				s.prev = prevDollar
			}
		case qOut:
			nested := len(s.stack) > 1
			switch {
			case c == '\\':
				s.esc = true
			case c == '\'':
				s.push(qSq)
			case c == '"':
				s.push(qDq)
			case c == '`':
				s.push(qTick)
			case c == '$' && next == '\'':
				s.push(qAnsi)
				i++
			case c == '$' && next == '(':
				s.push(qOut)
				s.prev = prevWordStart
				i++
			case c == '$' && next == '{':
				s.push(qBrace)
				i++
			case c == '$':
				s.prev = prevDollar
			case c == '#' && prev == prevWordStart:
				s.comment = true
			case c == '#' && prev == prevEither:
				s.unknown = "a # that may start a comment"
			case c == '<' && next == '<':
				s.unknown = "a here-document"
			case nested && (prev == prevWordStart || prev == prevEither) && strings.HasPrefix(text[i:], "case") &&
				(i+4 == len(text) || wordStartAfter(text[i+4])):
				s.unknown = "case inside $(…)" // its pattern's ) would end the $(…) early
			case nested && c == '(':
				top.depth++
			case nested && c == ')' && top.depth > 0:
				top.depth--
			case nested && c == ')':
				s.pop()
			}
			if s.prev == prevOther && wordStartAfter(c) {
				s.prev = prevWordStart
			}
		}
	}
	return s
}

// escaper is the function a placeholder at s gets, or why it cannot have one.
func (s shellState) escaper() (string, error) {
	switch {
	case s.unknown != "":
		return "", fmt.Errorf("placeholder after %s: lanes cannot tell its shell quoting", s.unknown)
	case s.esc:
		return "", errors.New("placeholder right after a backslash")
	case s.comment:
		return "", errors.New("placeholder inside a # comment")
	case s.prev == prevDollar:
		return "", errors.New("placeholder right after $")
	}
	switch s.stack[len(s.stack)-1].q {
	case qOut:
		return fnShq, nil
	case qDq:
		return fnDq, nil
	case qSq, qAnsi:
		return "", errors.New("placeholder inside single quotes; drop the quotes (values are quoted for you) or use \"…\"")
	case qTick:
		return "", errors.New("placeholder inside `…`; use $(…)")
	default:
		return "", errors.New("placeholder inside ${…}")
	}
}

// escapeShell adds the context's escaper to every placeholder of t.
func escapeShell(t *template.Template) error {
	if t.Tree == nil {
		return nil
	}
	_, err := escapeList(t.Tree, t.Tree.Root, shellState{stack: []frame{{q: qOut}}, prev: prevWordStart})
	return err
}

func escapeList(tree *parse.Tree, l *parse.ListNode, s shellState) (shellState, error) {
	if l == nil {
		return s, nil
	}
	var err error
	for _, n := range l.Nodes {
		switch n := n.(type) {
		case *parse.TextNode:
			s = s.scan(string(n.Text))
		case *parse.ActionNode:
			if len(n.Pipe.Decl) > 0 {
				continue // {{$x := …}} prints nothing
			}
			fn, err := s.escaper()
			if err != nil {
				loc, _ := tree.ErrorContext(n)
				return s, fmt.Errorf("%s %s: %w", loc, n, err)
			}
			escapePipe(n.Pipe, fn)
			s.prev = prevOther
		case *parse.IfNode:
			s, err = escapeBranch(tree, &n.BranchNode, s, "if")
		case *parse.WithNode:
			s, err = escapeBranch(tree, &n.BranchNode, s, "with")
		case *parse.RangeNode:
			s, err = escapeBranch(tree, &n.BranchNode, s, "range")
		case *parse.TemplateNode:
			loc, _ := tree.ErrorContext(n)
			return s, fmt.Errorf("%s %s: {{template}} is not supported in run", loc, n)
		}
		if err != nil {
			return s, err
		}
	}
	return s, nil
}

// escapeBranch escapes both lists of an if/with/range, which must end in the same quote context (a range body
// also in the one it started in, as it may run again).
func escapeBranch(tree *parse.Tree, b *parse.BranchNode, s shellState, kind string) (shellState, error) {
	a, err := escapeList(tree, b.List, s)
	if err != nil {
		return s, err
	}
	e, err := escapeList(tree, b.ElseList, s)
	if err != nil {
		return s, err
	}
	if !a.equal(e) || kind == "range" && !a.equal(s) {
		loc, _ := tree.ErrorContext(b)
		return s, fmt.Errorf("%s {{%s}}: the shell quoting differs between its branches", loc, kind)
	}
	if a.prev != e.prev {
		if a.prev == prevDollar || e.prev == prevDollar {
			a.unknown = "a $ in one branch only"
		} else {
			a.prev = prevEither
		}
	}
	return a, nil
}

// escapePipe makes p end in fn; a final q is replaced (so {{.x | q}} is {{.x}}), a final raw kept.
func escapePipe(p *parse.PipeNode, fn string) {
	last := p.Cmds[len(p.Cmds)-1]
	if id, ok := last.Args[0].(*parse.IdentifierNode); ok {
		switch id.Ident {
		case "raw":
			return
		case "q":
			id.Ident = fn
			return
		}
	}
	p.Cmds = append(p.Cmds, &parse.CommandNode{
		NodeType: parse.NodeCommand, Pos: last.Pos,
		Args: []parse.Node{parse.NewIdentifier(fn).SetTree(nil).SetPos(last.Pos)},
	})
}
