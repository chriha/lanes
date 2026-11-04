package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repo is a clone of a local bare "origin", both in a temp dir, with git isolated from the user's config.
type repo struct {
	t   *testing.T
	dir string
	env []string
}

func newRepo(t *testing.T) *repo {
	t.Helper()
	tmp := t.TempDir()
	r := &repo{t: t, dir: filepath.Join(tmp, "work"), env: []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + tmp,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + filepath.Join(tmp, "gitconfig"),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		"GIT_TERMINAL_PROMPT=0",
	}}
	r.run(tmp, "git", "init", "-q", "--bare", "-b", "main", "origin.git")
	r.run(tmp, "git", "clone", "-q", "origin.git", "work")
	r.git("checkout", "-q", "-B", "main")
	r.commit("chore: initial")
	r.git("push", "-q", "-u", "origin", "main")
	return r
}

func (r *repo) run(dir string, name string, args ...string) string {
	r.t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Env = dir, r.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return string(out)
}

func (r *repo) git(args ...string) string { r.t.Helper(); return r.run(r.dir, "git", args...) }

// commit makes an empty commit with message msg (a subject, optionally followed by a body).
func (r *repo) commit(msg string) { r.t.Helper(); r.git("commit", "-q", "--allow-empty", "-m", msg) }

// release tags HEAD as tag and pushes branch and tag.
func (r *repo) release(tag string) {
	r.t.Helper()
	r.git("tag", "-a", tag, "-m", tag)
	r.git("push", "-q", "origin", "main", tag)
}

func (r *repo) tags() string { r.t.Helper(); return strings.TrimSpace(r.git("tag", "--list")) }

func (r *repo) remoteTags() string {
	r.t.Helper()
	return strings.TrimSpace(r.git("ls-remote", "--tags", "origin"))
}

// script runs version.sh in the repo with the extra env vars and stdin, returning combined output and success.
func (r *repo) script(stdin string, env ...string) (string, bool) {
	r.t.Helper()
	path, err := filepath.Abs("version.sh")
	if err != nil {
		r.t.Fatal(err)
	}
	cmd := exec.Command(path)
	cmd.Dir, cmd.Env, cmd.Stdin = r.dir, append(append([]string{}, r.env...), env...), strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	return string(out), err == nil
}

func TestVersionNext(t *testing.T) {
	tests := []struct {
		name    string
		last    string   // tag released after the initial commit; "" = none
		commits []string // made after it, then pushed
		env     []string
		ok      bool
		want    string // in the output
	}{
		{"no tag is the first release", "", []string{"feat: a"}, nil, true, "v1.0.0"},
		{"fix only bumps patch", "v1.2.3", []string{"fix: a", "chore: b"}, nil, true, "v1.2.4"},
		{"feat bumps minor", "v1.2.3", []string{"fix: a", "feat(ui): b"}, nil, true, "v1.3.0"},
		{"bang subject refuses major", "v1.2.3", []string{"feat!: a"}, nil, false, "MAJOR=1"},
		{"scoped bang subject refuses major", "v1.2.3", []string{"fix(ui)!: a"}, nil, false, "MAJOR=1"},
		{"breaking footer refuses major", "v1.2.3", []string{"feat: a\n\nBREAKING CHANGE: gone"}, nil, false, "MAJOR=1"},
		{"major with MAJOR=1", "v1.2.3", []string{"feat!: a"}, []string{"MAJOR=1"}, true, "v2.0.0"},
		{"V override", "v1.2.3", []string{"fix: a"}, []string{"V=v1.5.0"}, true, "v1.5.0"},
		{"V override not semver", "v1.2.3", []string{"fix: a"}, []string{"V=1.5"}, false, "vX.Y.Z"},
		{"V override not greater", "v1.2.3", []string{"fix: a"}, []string{"V=v1.2.3"}, false, "greater"},
		{"V override major needs MAJOR=1", "v1.2.3", []string{"fix: a"}, []string{"V=v2.0.0"}, false, "MAJOR=1"},
		{"no commits since tag stops", "v1.2.3", nil, nil, false, "nothing to release"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRepo(t)
			if tt.last != "" {
				r.release(tt.last)
			}
			for _, c := range tt.commits {
				r.commit(c)
			}
			r.git("push", "-q", "origin", "main")
			before := r.tags()
			out, ok := r.script("", append([]string{"SKIP_CI=1", "DRY_RUN=1"}, tt.env...)...)
			if ok != tt.ok || !strings.Contains(out, tt.want) {
				t.Fatalf("ok = %v, want %v; output does not contain %q:\n%s", ok, tt.ok, tt.want, out)
			}
			if got := r.tags(); got != before {
				t.Errorf("tags = %q, want %q: a dry run or refusal must not tag", got, before)
			}
		})
	}
}

func TestVersionGuards(t *testing.T) {
	tests := []struct {
		name  string
		setup func(r *repo)
		want  string
	}{
		{"not on main", func(r *repo) { r.git("checkout", "-q", "-b", "other") }, "not on main"},
		{"dirty tree", func(r *repo) {
			if err := os.WriteFile(filepath.Join(r.dir, "x"), []byte("x"), 0o644); err != nil {
				r.t.Fatal(err)
			}
		}, "not clean"},
		{"ahead of origin", func(r *repo) { r.commit("fix: unpushed") }, "origin/main"},
		{"behind origin", func(r *repo) {
			r.commit("fix: pushed")
			r.git("push", "-q", "origin", "main")
			r.git("reset", "-q", "--hard", "HEAD~1")
		}, "origin/main"},
		{"tag exists", func(r *repo) {
			r.release("v1.2.3")
			r.commit("fix: a")
			r.git("push", "-q", "origin", "main")
			r.git("tag", "v1.2.4", "HEAD^{tree}") // not a release reachable from HEAD, but the name is taken
		}, "already exists"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRepo(t)
			r.commit("fix: a")
			r.git("push", "-q", "origin", "main")
			tt.setup(r)
			before := r.tags()
			out, ok := r.script("y\n", "SKIP_CI=1")
			if ok || !strings.Contains(out, tt.want) {
				t.Fatalf("ok = %v, want failure containing %q:\n%s", ok, tt.want, out)
			}
			if got := r.tags(); got != before {
				t.Errorf("tags = %q, want %q", got, before)
			}
		})
	}
}

func TestVersionConfirm(t *testing.T) {
	tests := []struct {
		answer string
		tagged bool
	}{
		{"y\n", true},
		{"n\n", false},
		{"yes\n", false},
		{"Y\n", false},
		{"\n", false},
		{"", false}, // stdin closed
	}
	for _, tt := range tests {
		t.Run(strings.TrimSpace(tt.answer), func(t *testing.T) {
			r := newRepo(t)
			r.release("v1.0.0")
			r.commit("feat: a")
			r.git("push", "-q", "origin", "main")
			out, ok := r.script(tt.answer, "SKIP_CI=1")
			if ok != tt.tagged {
				t.Fatalf("ok = %v, want %v:\n%s", ok, tt.tagged, out)
			}
			local := strings.Contains(r.tags(), "v1.1.0")
			remote := strings.Contains(r.remoteTags(), "refs/tags/v1.1.0")
			if local != tt.tagged || remote != tt.tagged {
				t.Errorf("v1.1.0 local = %v, pushed = %v, want %v:\n%s", local, remote, tt.tagged, out)
			}
		})
	}
}
