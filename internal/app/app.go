// Package app holds the app identity. Rename the tool by changing Name (plus go.mod and Makefile).
package app

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const Name = "lanes"

// scope separates the state of different projects (history, sessions, cache, tmux socket).
var scope string

var unsafe = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// Init derives the project scope from the git root of dir (or dir itself outside git).
func Init(dir string) {
	root := dir
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output(); err == nil {
		root = strings.TrimSpace(string(out))
	}
	sum := sha1.Sum([]byte(root))
	scope = unsafe.ReplaceAllString(filepath.Base(root), "_") + "-" + hex.EncodeToString(sum[:])[:6]
}

// Socket is the tmux server name, one per project so session names never collide across projects.
func Socket() string {
	if scope == "" {
		return Name
	}
	return Name + "-" + scope
}

func ConfigDir() string { return xdg("XDG_CONFIG_HOME", ".config") }
func CacheDir() string  { return filepath.Join(xdg("XDG_CACHE_HOME", ".cache"), scope) }
func StateDir() string  { return filepath.Join(StateRoot(), scope) }

// StateRoot is the state dir shared by all projects.
func StateRoot() string { return xdg("XDG_STATE_HOME", ".local/state") }

func xdg(env, fallback string) string {
	base := os.Getenv(env)
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, fallback)
	}
	return filepath.Join(base, Name)
}
