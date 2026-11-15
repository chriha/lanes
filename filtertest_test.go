package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/chriha/lanes/internal/cache"
	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/trust"
)

func TestParseFilterTest(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantExpr string
		wantView string
		wantOK   bool
	}{
		{"expression", []string{"test", "draft"}, "draft", "", true},
		{"empty expression", []string{"test", ""}, "", "", true},
		{"view after", []string{"test", "draft", "--view", "review"}, "draft", "review", true},
		{"view before", []string{"test", "--view", "review", "draft"}, "draft", "review", true},
		{"no args", nil, "", "", false},
		{"unknown subcommand", []string{"run", "draft"}, "", "", false},
		{"no expression", []string{"test"}, "", "", false},
		{"two expressions", []string{"test", "a", "b"}, "", "", false},
		{"view without name", []string{"test", "draft", "--view"}, "", "", false},
		{"view twice", []string{"test", "a", "--view", "x", "--view", "y"}, "", "", false},
		{"unknown flag", []string{"test", "a", "--json"}, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, view, ok := parseFilterTest(tt.args)
			if ok != tt.wantOK || expr != tt.wantExpr || view != tt.wantView {
				t.Errorf("parseFilterTest(%q) = %q, %q, %v; want %q, %q, %v", tt.args, expr, view, ok, tt.wantExpr, tt.wantView, tt.wantOK)
			}
		})
	}
}

func TestUsageErrorFilter(t *testing.T) {
	if got := usageError([]string{"filter", "test", "draft"}); got != "" {
		t.Errorf("valid filter test: %q", got)
	}
	for _, args := range [][]string{{"filter"}, {"filter", "test"}, {"filter", "x"}, {"filter", "test", "a", "b"}} {
		if got := usageError(args); !strings.HasPrefix(got, "usage: lanes filter test") {
			t.Errorf("usageError(%q) = %q, want the filter usage", args, got)
		}
	}
}

func TestRunFilterTest(t *testing.T) {
	cfg := &config.Config{
		Provider: map[string]config.Provider{"a": {}, "b": {}},
		View: []config.View{
			{Name: "mine", Provider: "a", Filter: "draft"}, // its own filter is not applied
			{Name: "other", Provider: "b"},
		},
	}
	load := func() (*config.Config, error) { return cfg, nil }
	items := func(name string) []item.Item {
		if name == "none" {
			return nil
		}
		if name == "b" {
			return []item.Item{{ID: "9", Title: "Nine", Author: "zed"}}
		}
		return []item.Item{
			{ID: "1", Title: "One", Author: "ann"},
			{ID: "2", Title: "Two", Author: "bob", Draft: true},
		}
	}
	tests := []struct {
		name       string
		load       func() (*config.Config, error)
		expr, view string
		wantCode   int
		wantOut    []string // each must appear in stdout, in order
		wantErr    string   // substring of stderr; "" = stderr must be empty
	}{
		{"first view, no failing clause", load, `author == "ann" || author == "bob"`, "", 0,
			[]string{"match  1 One", "match  2 Two", "2 of 2 items match"}, ""},
		{"and names the failed operand", load, `author == "ann" && !draft`, "", 0,
			[]string{"match  1 One", "no match  2 Two", "  failed: !draft", "1 of 2 items match"}, ""},
		{"or names every operand", load, `author == "x" || draft == false`, "", 0,
			[]string{"no match  2 Two", "  failed: author == \"x\"", "  failed: draft == false"}, ""},
		{"not names its operand", load, `!draft`, "", 0, []string{"match  1 One", "no match  2 Two", "  failed: draft"}, ""},
		{"nothing matches exits 0", load, `author == "nobody"`, "", 0, []string{"no match  1 One", "0 of 2 items match"}, ""},
		{"view flag picks the provider", load, `author == "zed"`, "other", 0, []string{"match  9 Nine", "1 of 1 items match"}, ""},
		{"no cached items", func() (*config.Config, error) {
			return &config.Config{View: []config.View{{Name: "v", Provider: "none"}}}, nil
		}, `draft`, "", 0, []string{"no cached items"}, ""},
		{"compile error", load, `author ==`, "", 2, nil, "author =="},
		{"compile error wins over a bad config", func() (*config.Config, error) { return nil, errors.New("boom") }, `a &&`, "", 2, nil, "a &&"},
		{"unknown view names the configured ones", load, `draft`, "nope", 1, nil, `unknown view "nope"; views: mine, other`},
		{"config error", func() (*config.Config, error) { return nil, errors.New("boom") }, `draft`, "", 1, nil, "boom"},
		{"no views", func() (*config.Config, error) { return &config.Config{}, nil }, `draft`, "", 1, nil, "no views"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			code := runFilterTest(&out, &errb, tt.load, items, tt.expr, tt.view)
			if code != tt.wantCode {
				t.Errorf("code = %d, want %d (stdout %q, stderr %q)", code, tt.wantCode, out.String(), errb.String())
			}
			rest := out.String()
			for _, w := range tt.wantOut {
				i := strings.Index(rest, w)
				if i < 0 {
					t.Errorf("stdout lacks %q in order:\n%s", w, out.String())
					break
				}
				rest = rest[i+len(w):]
			}
			if tt.wantErr == "" && errb.Len() > 0 || !strings.Contains(errb.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", errb.String(), tt.wantErr)
			}
		})
	}
}

// TestRunFilterTestReadsTheCache runs against the real cache in a temp dir: the items come from disk.
func TestRunFilterTestReadsTheCache(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cache.Save("p", []item.Item{{ID: "7", Title: "Seven", Author: "ann", Fields: map[string]any{"labels": []any{"x"}}}})
	cfg := &config.Config{View: []config.View{{Name: "v", Provider: "p"}}}
	var out, errb bytes.Buffer
	code := runFilterTest(&out, &errb, func() (*config.Config, error) { return cfg, nil }, cache.Load, `hasAny(labels, "y")`, "")
	if code != 0 || errb.Len() != 0 {
		t.Fatalf("code %d, stderr %q", code, errb.String())
	}
	if want := "no match  7 Seven"; !strings.Contains(out.String(), want) {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}

func TestRunFilterGate(t *testing.T) {
	tests := []struct {
		name      string
		trusted   bool
		wantCode  int
		wantCalls string
	}{
		{"untrusted, no terminal", false, 1, ""},
		{"trusted", true, 0, "initApp filterTest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			root, dir := gateRoot(t)
			if tt.trusted {
				hash, _ := trust.HashRoot(root)
				if err := trust.Trust(root, hash); err != nil {
					t.Fatal(err)
				}
			}
			var calls []string
			e, _, _ := fakeEnv(dir, false, true, &calls)
			e.filterTest = func(expr, view string) int {
				calls = append(calls, "filterTest")
				if expr != "draft" || view != "v" {
					t.Errorf("filterTest(%q, %q)", expr, view)
				}
				return 0
			}
			if code := run([]string{"filter", "test", "draft", "--view", "v"}, e); code != tt.wantCode {
				t.Errorf("code = %d, want %d", code, tt.wantCode)
			}
			if got := strings.Join(calls, " "); got != tt.wantCalls {
				t.Errorf("calls = %q, want %q", got, tt.wantCalls)
			}
		})
	}
}
