package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// ErrNotSavable means SaveViewFilter could not place the filter safely and left the file unchanged.
var ErrNotSavable = errors.New("filter can't be saved")

// ViewFile returns the config file whose [[view]] named name is in effect: the project file when it
// defines that view (it replaces the global one), else the global file.
func ViewFile(name string) (string, error) {
	pp := ProjectPath()
	if pp == "" {
		return Path(), nil
	}
	var pc Config
	if _, err := toml.DecodeFile(pp, &pc); err != nil {
		return "", fmt.Errorf("%s: %w", pp, err)
	}
	for _, v := range pc.View {
		if v.Name == name {
			return pp, nil
		}
	}
	return Path(), nil
}

var (
	nameLine   = regexp.MustCompile(`^\s*name\s*=`)
	filterLine = regexp.MustCompile(`^\s*filter\s*=`)
)

// SaveViewFilter writes expr as the filter of the [[view]] named view in the TOML file at path. It edits
// only that view's filter line (or adds one at the end of its block), so comments and layout stay. When
// the result would not decode to the same config with just that filter changed (multi-line filter, inline
// table views, no such view, ...), it returns ErrNotSavable and leaves the file unchanged.
func SaveViewFilter(path, view, expr string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.SplitAfter(string(b), "\n")
	value := "'" + expr + "'"
	if strings.Contains(expr, "'") { // a literal string can't hold '
		value = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(expr) + `"`
	}
	// ponytail: line scan, no TOML parser; the decode check below refuses whatever it gets wrong
	start, matched := -1, false
	edit := func(end int) bool { // edits the block lines[start:end] when it is the view's
		if !matched {
			return false
		}
		last := start // last key line of the block; a new filter goes after it
		for i := start; i < end; i++ {
			t := strings.TrimSpace(lines[i])
			if filterLine.MatchString(lines[i]) {
				lines[i] = withEOL("filter = "+value, lines[i])
				return true
			}
			if t != "" && !strings.HasPrefix(t, "#") {
				last = i
			}
		}
		if !strings.HasSuffix(lines[last], "\n") {
			lines[last] += "\n"
		}
		lines = append(lines[:last+1], append([]string{withEOL("filter = "+value, lines[last])}, lines[last+1:]...)...)
		return true
	}
	done := false
	for i := 0; i <= len(lines) && !done; i++ {
		t := ""
		if i < len(lines) {
			t = strings.TrimSpace(lines[i])
		}
		switch {
		case i == len(lines):
			done = edit(i)
		case strings.HasPrefix(t, "["): // a table header ends the block
			if done = edit(i); !done {
				head, _, _ := strings.Cut(t, "#")
				start, matched = -1, false
				if strings.ReplaceAll(head, " ", "") == "[[view]]" {
					start = i
				}
			}
		case start >= 0 && nameLine.MatchString(lines[i]):
			var v View
			_, err := toml.Decode(lines[i], &v)
			matched = err == nil && v.Name == view
		}
	}
	out := strings.Join(lines, "")
	if !done || !onlyFilterChanged(string(b), out, view, expr) {
		return fmt.Errorf("%s: %w", path, ErrNotSavable)
	}
	return os.WriteFile(path, []byte(out), 0o644)
}

// withEOL is s with the line ending of line.
func withEOL(s, line string) string { return s + line[len(strings.TrimRight(line, "\r\n")):] }

// onlyFilterChanged reports whether after decodes to before with only view's filter set to expr.
func onlyFilterChanged(before, after, view, expr string) bool {
	var want, got map[string]any
	if _, err := toml.Decode(before, &want); err != nil {
		return false
	}
	if _, err := toml.Decode(after, &got); err != nil {
		return false
	}
	views, _ := want["view"].([]map[string]any)
	for _, v := range views {
		if v["name"] == view {
			v["filter"] = expr
		}
	}
	return reflect.DeepEqual(want, got)
}
