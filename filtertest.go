package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/filter"
	"github.com/chriha/lanes/internal/item"
)

// filterUsage is the usage of `lanes filter`.
const filterUsage = "usage: " + app.Name + " filter test '<expr>' [--view <name>]"

// parseFilterTest reads the arguments after `filter`: `test`, one expression and an optional `--view <name>`.
func parseFilterTest(args []string) (expr, view string, ok bool) {
	if len(args) == 0 || args[0] != "test" {
		return "", "", false
	}
	var gotExpr, gotView bool
	for i := 1; i < len(args); i++ {
		switch {
		case args[i] == "--view":
			if gotView || i+1 == len(args) {
				return "", "", false
			}
			i++
			view, gotView = args[i], true
		case strings.HasPrefix(args[i], "--"):
			return "", "", false
		case gotExpr:
			return "", "", false
		default:
			expr, gotExpr = args[i], true
		}
	}
	return expr, view, gotExpr
}

// runFilterTest prints, for each cached item of the view's provider, whether expr matches it and which top-level
// clause failed when it does not. The view's own filter is not applied. It returns 2 when expr does not compile and
// 1 when the config or the view is unusable; items that match nothing are no error.
func runFilterTest(stdout, stderr io.Writer, load func() (*config.Config, error), cached func(name string) []item.Item, expr, view string) int {
	f, err := filter.Compile(expr)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", app.Name, err)
		return 2
	}
	cfg, err := load()
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", app.Name, err)
		return 1
	}
	if len(cfg.View) == 0 {
		fmt.Fprintf(stderr, "%s: no views configured\n", app.Name)
		return 1
	}
	v := cfg.View[0]
	if view != "" {
		found := false
		names := make([]string, len(cfg.View))
		for i, c := range cfg.View {
			names[i] = c.Name
			if c.Name == view {
				v, found = c, true
			}
		}
		if !found {
			fmt.Fprintf(stderr, "%s: unknown view %q; views: %s\n", app.Name, view, strings.Join(names, ", "))
			return 1
		}
	}
	items := item.Sanitize(cached(v.Provider))
	if len(items) == 0 {
		fmt.Fprintf(stdout, "no cached items for view %s (provider %s); start %s once to fill the cache\n", v.Name, v.Provider, app.Name)
		return 0
	}
	matched := 0
	for _, it := range items {
		env := it.Env()
		ok, err := f.Match(env)
		var failed []string
		if err == nil && !ok {
			failed, err = filter.Failed(expr, env)
		}
		switch {
		case err != nil:
			fmt.Fprintf(stderr, "%s: item %s: %v\n", app.Name, it.ID, err)
		case ok:
			matched++
			fmt.Fprintf(stdout, "match  %s %s\n", it.ID, it.Title)
		default:
			fmt.Fprintf(stdout, "no match  %s %s\n", it.ID, it.Title)
			for _, c := range failed {
				fmt.Fprintf(stdout, "  failed: %s\n", c)
			}
		}
	}
	fmt.Fprintf(stdout, "%d of %d items match\n", matched, len(items))
	return 0
}
