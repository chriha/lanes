package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/filter"
	"github.com/chriha/lanes/internal/provider"
	"github.com/chriha/lanes/internal/ui"
)

type namedDir struct{ name, path string }

// doctorEnv holds the lookups `lanes doctor` makes, so tests never need tmux, glab, gh, git or the real dirs.
type doctorEnv struct {
	lookPath  func(string) (string, error)
	run       func(name string, args ...string) (string, error) // combined output
	getenv    func(string) string
	load      func() (*config.Config, error)
	overrides func() (*config.Overrides, error)
	dirs      []namedDir
}

func realDoctorEnv() doctorEnv {
	return doctorEnv{
		lookPath: exec.LookPath,
		getenv:   os.Getenv,
		run: func(name string, args ...string) (string, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
			return string(out), err
		},
		load:      config.Load,
		overrides: config.LoadOverrides,
		dirs:      []namedDir{{"state dir", app.StateDir()}, {"cache dir", app.CacheDir()}},
	}
}

var tmuxVersion = regexp.MustCompile(`(\d+)\.(\d+)`)

// runDoctor prints one line per check and returns the exit code: 1 if any check failed.
func runDoctor(w io.Writer, e doctorEnv) int {
	code := 0
	line := func(status, name, msg string) {
		if status == "fail" {
			code = 1
		}
		fmt.Fprintf(w, "%-4s  %s: %s\n", status, name, msg)
	}

	if _, err := e.lookPath("tmux"); err != nil {
		line("fail", "tmux", "not found in PATH (install tmux ≥ 3.2)")
	} else if out, err := e.run("tmux", "-V"); err != nil {
		line("fail", "tmux", firstLine(out, err))
	} else if m := tmuxVersion.FindStringSubmatch(out); m == nil {
		line("warn", "tmux", "cannot read version from "+strconv.Quote(strings.TrimSpace(out)))
	} else if atoi(m[1])*100+atoi(m[2]) < 302 {
		line("warn", "tmux", strings.TrimSpace(out)+" (lanes needs ≥ 3.2)")
	} else {
		line("ok", "tmux", strings.TrimSpace(out))
	}

	cfg, err := e.load()
	if err != nil {
		line("fail", "config", err.Error())
	} else {
		line("ok", "config", "loaded")
		checkForgeCLIs(e, cfg, line)
		checkTokens(e, cfg, line)
		ov, err := e.overrides()
		if err != nil {
			line("fail", "overrides", err.Error())
		}
		checkViews(cfg, ov, line)
		names := make([]string, 0, len(cfg.Action))
		for n := range cfg.Action {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			a := cfg.Action[n]
			if _, err := ui.ParseTemplate(a.Run); err != nil {
				line("fail", "action "+n, "run: "+err.Error())
			} else if _, err := ui.ParseCwdTemplate(a.Cwd); err != nil {
				line("fail", "action "+n, "cwd: "+err.Error())
			} else {
				line("ok", "action "+n, "template parses")
			}
		}
	}

	for _, d := range e.dirs {
		if err := writable(d.path); err != nil {
			line("fail", d.name, err.Error())
		} else {
			line("ok", d.name, d.path)
		}
	}
	return code
}

// checkForgeCLIs checks the CLI (glab, gh) of each forge a provider uses; an auto provider resolves its forge
// from the current directory's origin remote, as the TUI would.
func checkForgeCLIs(e doctorEnv, cfg *config.Config, line func(status, name, msg string)) {
	names := make([]string, 0, len(cfg.Provider))
	for n := range cfg.Provider {
		names = append(names, n)
	}
	sort.Strings(names)
	used := map[string]bool{}
	for _, n := range names {
		typ := cfg.Provider[n].Type
		if typ == "auto" {
			out, err := e.run("git", "remote", "get-url", "origin")
			if err != nil {
				line("warn", "provider "+n, "auto: no git remote origin here, cannot tell github from gitlab")
				continue
			}
			typ, _ = provider.Forge(out)
		}
		used[typ] = true
	}
	checkCLI(e, used["gitlab"], "glab", "gitlab", line)
	checkCLI(e, used["github"] || used["github-issues"], "gh", "github", line)
}

// checkTokens reports whether each linear provider has a team and its API key env var is set; it never prints the key.
func checkTokens(e doctorEnv, cfg *config.Config, line func(status, name, msg string)) {
	names := make([]string, 0, len(cfg.Provider))
	for n := range cfg.Provider {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		p := cfg.Provider[n]
		if p.Type != "linear" {
			continue
		}
		env := (&provider.Linear{TokenEnv: p.TokenEnv}).TokenVar()
		if p.Team == "" {
			line("fail", "provider "+n, "team is not set (provider.team, e.g. \"ENG\")")
		} else if e.getenv(env) == "" {
			line("fail", "provider "+n, env+" is not set (a Linear API key)")
		} else {
			line("ok", "provider "+n, env+" is set")
		}
	}
}

func checkCLI(e doctorEnv, needed bool, cli, typ string, line func(status, name, msg string)) {
	if !needed {
		line("ok", cli, "skipped, no "+typ+" provider configured")
		return
	}
	if _, err := e.lookPath(cli); err != nil {
		line("fail", cli, "not found in PATH (needed by the "+typ+" provider)")
		return
	}
	if out, err := e.run(cli, "auth", "status"); err != nil {
		line("fail", cli+" auth", firstLine(out, err)+" (run "+cli+" auth login)")
		return
	}
	line("ok", cli, "logged in")
}

// checkViews compiles the filters the TUI would use: config views with their :filter overrides,
// plus views added with :newview.
func checkViews(cfg *config.Config, ov *config.Overrides, line func(status, name, msg string)) {
	views := append([]config.View{}, cfg.View...)
	for i, v := range views {
		if f, ok := ov.Filters[v.Name]; ok {
			views[i].Filter = f
		}
	}
	seen := map[string]bool{}
	for _, v := range cfg.View {
		seen[v.Name] = true
	}
	for _, v := range ov.Views {
		if _, known := cfg.Provider[v.Provider]; known && !seen[v.Name] {
			views = append(views, v)
		}
	}
	for _, v := range views {
		if _, err := filter.Compile(v.Filter); err != nil {
			line("fail", "view "+v.Name, "filter: "+err.Error())
		} else {
			line("ok", "view "+v.Name, "filter compiles")
		}
	}
}

func writable(dir string) error {
	if err := os.MkdirAll(dir, app.DirMode); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".doctor-*")
	if err != nil {
		return err
	}
	return errors.Join(f.Close(), os.Remove(f.Name()))
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s) // the regexp guarantees digits
	return n
}

func firstLine(out string, err error) string {
	if s, _, _ := strings.Cut(strings.TrimSpace(out), "\n"); s != "" {
		return s
	}
	return err.Error()
}
