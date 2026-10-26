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
	"github.com/chriha/lanes/internal/ui"
)

type namedDir struct{ name, path string }

// doctorEnv holds the lookups `lanes doctor` makes, so tests never need tmux, glab or the real dirs.
type doctorEnv struct {
	lookPath  func(string) (string, error)
	run       func(name string, args ...string) (string, error) // combined output
	load      func() (*config.Config, error)
	overrides func() (*config.Overrides, error)
	dirs      []namedDir
}

func realDoctorEnv() doctorEnv {
	return doctorEnv{
		lookPath: exec.LookPath,
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
		checkGlab(e, cfg, line)
		ov, _ := e.overrides() // stub
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
			} else if _, err := ui.ParseTemplate(a.Cwd); err != nil {
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

func checkGlab(e doctorEnv, cfg *config.Config, line func(status, name, msg string)) {
	gitlab := false
	for _, p := range cfg.Provider {
		gitlab = gitlab || p.Type == "gitlab"
	}
	if !gitlab {
		line("ok", "glab", "skipped, no gitlab provider configured")
		return
	}
	if _, err := e.lookPath("glab"); err != nil {
		line("fail", "glab", "not found in PATH (needed by the gitlab provider)")
		return
	}
	if out, err := e.run("glab", "auth", "status"); err != nil {
		line("fail", "glab auth", firstLine(out, err)+" (run glab auth login)")
		return
	}
	line("ok", "glab", "logged in")
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
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
