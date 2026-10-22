package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestXDG(t *testing.T) {
	home := t.TempDir()
	custom := t.TempDir()
	tests := []struct {
		name string
		set  bool
		want string
	}{
		{"env set", true, filepath.Join(custom, Name)},
		{"env unset", false, filepath.Join(home, ".cfg", Name)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", home)
			t.Setenv("XDG_TEST_HOME", "")
			if tt.set {
				t.Setenv("XDG_TEST_HOME", custom)
			}
			if got := xdg("XDG_TEST_HOME", ".cfg"); got != tt.want {
				t.Errorf("xdg = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDirs(t *testing.T) {
	home := t.TempDir()
	xdgRoot := t.TempDir()
	envs := map[string]string{"XDG_CONFIG_HOME": ".config", "XDG_CACHE_HOME": ".cache", "XDG_STATE_HOME": ".local/state"}
	dirs := map[string]func() string{"XDG_CONFIG_HOME": ConfigDir, "XDG_CACHE_HOME": CacheDir, "XDG_STATE_HOME": StateDir}

	old := scope
	t.Cleanup(func() { scope = old })
	Init(t.TempDir())

	for _, set := range []bool{true, false} {
		t.Setenv("HOME", home)
		for env, fallback := range envs {
			base := filepath.Join(home, fallback)
			if set {
				base = filepath.Join(xdgRoot, env)
				t.Setenv(env, base)
			} else {
				t.Setenv(env, "")
			}
			got := dirs[env]()
			if !strings.HasPrefix(got, filepath.Join(base, Name)) {
				t.Errorf("%s (set=%v) = %q, want prefix %q", env, set, got, filepath.Join(base, Name))
			}
		}
	}

}

func TestDirsPerProject(t *testing.T) {
	xdgRoot := t.TempDir()
	old := scope
	t.Cleanup(func() { scope = old })
	// config is global, cache and state are per project
	t.Setenv("XDG_CONFIG_HOME", xdgRoot)
	t.Setenv("XDG_CACHE_HOME", xdgRoot)
	t.Setenv("XDG_STATE_HOME", xdgRoot)
	Init(t.TempDir())
	c1, ca1, s1 := ConfigDir(), CacheDir(), StateDir()
	Init(t.TempDir())
	if ConfigDir() != c1 {
		t.Errorf("ConfigDir changed with project: %q vs %q", ConfigDir(), c1)
	}
	if CacheDir() == ca1 || StateDir() == s1 {
		t.Errorf("cache/state dirs not per project: %q %q", ca1, s1)
	}
}

func TestSocket(t *testing.T) {
	old := scope
	t.Cleanup(func() { scope = old })

	scope = ""
	if got := Socket(); got != Name {
		t.Errorf("uninitialised Socket = %q, want %q", got, Name)
	}

	a, b := t.TempDir(), t.TempDir()
	Init(a)
	first := Socket()
	Init(a)
	if again := Socket(); again != first {
		t.Errorf("Socket not stable for one dir: %q vs %q", first, again)
	}
	Init(b)
	if other := Socket(); other == first {
		t.Errorf("Socket equal for different dirs: %q", other)
	}
	if !strings.HasPrefix(first, Name+"-") {
		t.Errorf("Socket %q lacks %q prefix", first, Name+"-")
	}
}
