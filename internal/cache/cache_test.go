package cache

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chriha/lanes/internal/item"
)

func TestSaveLoad(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	a := []item.Item{{
		ID: "1", Title: "one", URL: "u1", Author: "al", Draft: true,
		Badges:  []item.Badge{{Text: "ci", Kind: "ok"}},
		Fields:  map[string]any{"n": float64(3), "s": "x"},
		Pending: []string{"ci"},
	}}
	b := []item.Item{{ID: "2", Title: "two"}}
	Save("a", a)
	Save("b", b)

	for _, tc := range []struct {
		name string
		want []item.Item
	}{{"a", a}, {"b", b}} {
		if got := Load(tc.name); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Load(%q) = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestLoadMissingOrCorrupt(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if got := Load("nope"); got != nil {
		t.Errorf("missing: got %+v, want nil", got)
	}
	if err := os.MkdirAll(filepath.Dir(path("x")), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"bad": "{not json", "wrongshape": `{"id":"1"}`, "empty": ""} {
		if err := os.WriteFile(path(name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := Load(name); got != nil {
			t.Errorf("%s: got %+v, want nil", name, got)
		}
	}
}

func TestSaveIsPrivate(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Dir(path("a")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path("a"), []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	Save("a", []item.Item{{ID: "1"}})
	if fi, err := os.Stat(path("a")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("rewritten file = %v, %v; want 0600", fi, err)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	Save("b", []item.Item{{ID: "1"}})
	if fi, err := os.Stat(filepath.Dir(path("b"))); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("new dir = %v, %v; want 0700", fi, err)
	}
}
