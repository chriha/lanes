package ui

import (
	"context"
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/item"
)

type quickProv struct{ quick, full int }

func (p *quickProv) Fetch(context.Context) ([]item.Item, error) { p.full++; return nil, nil }
func (p *quickProv) Quick(context.Context) ([]item.Item, error) { p.quick++; return nil, nil }

// runCmd runs c and, for a tea.Sequence, each command it holds, returning the messages in order.
func runCmd(c tea.Cmd) []tea.Msg {
	msg := c()
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for i := 0; i < v.Len(); i++ {
		if sub, ok := v.Index(i).Interface().(tea.Cmd); ok {
			out = append(out, sub())
		}
	}
	return out
}

func TestFetchQuickPassOnlyWithoutFullResult(t *testing.T) {
	m := newBackoffModel(t)
	p := &quickProv{}
	m.providers["p"] = p

	runCmd(m.fetch("p"))
	if p.quick != 1 || p.full != 1 {
		t.Fatalf("first fetch: quick=%d full=%d, want 1 and 1", p.quick, p.full)
	}

	m.Update(fetchedMsg{name: "p", items: mkItems("a")}) // the full result lands
	runCmd(m.fetch("p"))
	if p.quick != 1 || p.full != 2 {
		t.Errorf("poll after full result: quick=%d full=%d, want 1 and 2", p.quick, p.full)
	}

	m.Update(fetchedMsg{name: "p", items: mkItems("a")})
	m.applyConfig(m.cfg) // provider rebuilt: partial result first again
	p2 := &quickProv{}
	m.providers["p"] = p2
	runCmd(m.fetch("p"))
	if p2.quick != 1 || p2.full != 1 {
		t.Errorf("after config reload: quick=%d full=%d, want 1 and 1", p2.quick, p2.full)
	}
}
