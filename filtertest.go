package main

import (
	"io"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/item"
)

// filterUsage is the usage of `lanes filter`.
const filterUsage = "usage: lanes filter test '<expr>' [--view <name>]"

// parseFilterTest reads the arguments after `filter`: `test`, one expression and an optional `--view <name>`.
func parseFilterTest(args []string) (expr, view string, ok bool) {
	return "", "", false
}

// runFilterTest prints, for each cached item of the view's provider, whether expr matches it and which top-level
// clause failed when it does not. It returns 2 when expr does not compile and 1 when it cannot run at all.
func runFilterTest(stdout, stderr io.Writer, load func() (*config.Config, error), cached func(name string) []item.Item, expr, view string) int {
	return 0
}
