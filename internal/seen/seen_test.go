package seen

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/chriha/lanes/internal/app"
)

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	app.Init(t.TempDir())
}

func load(t *testing.T) (Store, bool) {
	t.Helper()
	s, first, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s, first
}

func TestUnread(t *testing.T) {
	s := Store{"!1": "a"}
	tests := []struct {
		name, id, fp string
		want         bool
	}{
		{"same fingerprint", "!1", "a", false},
		{"changed fingerprint", "!1", "b", true},
		{"never seen", "!2", "a", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.Unread(tt.id, tt.fp); got != tt.want {
				t.Errorf("Unread(%q, %q) = %v, want %v", tt.id, tt.fp, got, tt.want)
			}
		})
	}
}

func TestFirstRun(t *testing.T) {
	setup(t)
	s, first := load(t)
	if !first || len(s) != 0 {
		t.Fatalf("without seen.json: first = %v, store = %v; want true, empty", first, s)
	}
	if err := s.MarkRead(map[string]string{"!1": "a", "#2": "b"}); err != nil {
		t.Fatal(err)
	}
	got, first := load(t)
	if first {
		t.Fatal("first = true after seen.json was written")
	}
	if got.Unread("!1", "a") || got.Unread("#2", "b") {
		t.Fatalf("items recorded on the first run read as unread: %v", got)
	}
}

func TestMarkReadUnreadSurviveReload(t *testing.T) {
	setup(t)
	s, _ := load(t)
	if err := s.MarkRead(map[string]string{"!1": "a", "!2": "b", "!3": "c"}); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkRead(map[string]string{"!1": "a2"}); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkUnread("!2", "!missing"); err != nil {
		t.Fatal(err)
	}
	got, _ := load(t)
	if want := (Store{"!1": "a2", "!3": "c"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("after reload got %v, want %v", got, want)
	}
}

func TestSavesOnlyOnChange(t *testing.T) {
	setup(t)
	s, _ := load(t)
	if err := s.MarkRead(map[string]string{"!1": "a"}); err != nil {
		t.Fatal(err)
	}
	old := time.Unix(1, 0)
	if err := os.Chtimes(path(), old, old); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		op    func() error
		write bool
	}{
		{"read again, same fingerprint", func() error { return s.MarkRead(map[string]string{"!1": "a"}) }, false},
		{"unread an unknown item", func() error { return s.MarkUnread("!2") }, false},
		{"nothing to mark", func() error { return s.MarkRead(nil) }, false},
		{"read with a new fingerprint", func() error { return s.MarkRead(map[string]string{"!1": "b"}) }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.Chtimes(path(), old, old); err != nil {
				t.Fatal(err)
			}
			if err := tt.op(); err != nil {
				t.Fatal(err)
			}
			fi, err := os.Stat(path())
			if err != nil {
				t.Fatal(err)
			}
			if wrote := !fi.ModTime().Equal(old); wrote != tt.write {
				t.Errorf("wrote seen.json = %v, want %v", wrote, tt.write)
			}
		})
	}
}

func TestSaveIsAtomic(t *testing.T) {
	setup(t)
	s, _ := load(t)
	if err := s.MarkRead(map[string]string{"a": "x"}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(path()))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "seen.json" {
		t.Fatalf("want only seen.json in the state dir, got %v", entries)
	}
}

func TestCorruptFileIsReportedAndKept(t *testing.T) {
	setup(t)
	if err := os.MkdirAll(filepath.Dir(path()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path(), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, first, err := Load()
	if err == nil {
		t.Fatal("want an error for a corrupt seen.json")
	}
	if first {
		t.Fatal("a corrupt seen.json counts as the first run")
	}
	if s == nil || len(s) != 0 {
		t.Fatalf("want an empty usable store, got %v", s)
	}
	if b, _ := os.ReadFile(path()); string(b) != "{not json" {
		t.Fatalf("Load changed the corrupt file: %q", b)
	}
	if err := s.MarkRead(map[string]string{"a": "x"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path() + ".bak"); string(b) != "{not json" {
		t.Fatalf("want the corrupt file kept as seen.json.bak, got %q", b)
	}
	if got, _ := load(t); got["a"] != "x" {
		t.Fatal("seen state not saved")
	}
}

func TestUnreadableFileIsNotBackedUp(t *testing.T) {
	setup(t)
	s, _ := load(t)
	if err := s.MarkRead(map[string]string{"a": "x"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path(), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(path()); err == nil {
		t.Skip("file still readable (running as root)")
	}
	if err := (Store{"b": "y"}).MarkRead(map[string]string{"c": "z"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path() + ".bak"); err == nil {
		t.Fatal("a read error moved seen.json aside as if it were corrupt")
	}
}

func TestSaveDirIsPrivate(t *testing.T) {
	setup(t)
	s, _ := load(t)
	if err := s.MarkRead(map[string]string{"a": "x"}); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]os.FileMode{filepath.Dir(path()): 0o700, path(): 0o600} {
		if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s = %v, %v; want %o", p, fi, err, want)
		}
	}
}
