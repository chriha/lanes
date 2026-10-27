package notes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chriha/lanes/internal/app"
)

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	app.Init(t.TempDir())
}

func file() string { return filepath.Join(app.StateDir(), "notes.json") }

func load(t *testing.T) Store {
	t.Helper()
	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

func TestLoadMissingFile(t *testing.T) {
	setup(t)
	if s := load(t); len(s) != 0 {
		t.Fatalf("want no notes, got %v", s)
	}
}

func TestSetGetSurvivesReload(t *testing.T) {
	setup(t)
	s := load(t)
	if err := s.Set("!12", "waiting on backend\nre-review Fri"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("#3", "other"); err != nil {
		t.Fatal(err)
	}
	tests := []struct{ id, want string }{
		{"!12", "waiting on backend\nre-review Fri"},
		{"#3", "other"},
		{"!99", ""},
	}
	for _, tt := range tests {
		if got := s.Get(tt.id); got != tt.want {
			t.Errorf("Get(%q) = %q, want %q", tt.id, got, tt.want)
		}
		if got := load(t).Get(tt.id); got != tt.want {
			t.Errorf("after reload Get(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
	if load(t)["!12"].Updated.IsZero() {
		t.Error("Set did not record the update time")
	}
}

func TestEmptyTextDeletes(t *testing.T) {
	for _, text := range []string{"", "  \n\t\n"} {
		setup(t)
		s := load(t)
		if err := s.Set("a", "keep"); err != nil {
			t.Fatal(err)
		}
		if err := s.Set("a", text); err != nil {
			t.Fatal(err)
		}
		if _, ok := load(t)["a"]; ok {
			t.Errorf("Set(%q) kept the note", text)
		}
	}
}

func TestDelete(t *testing.T) {
	setup(t)
	s := load(t)
	if err := s.Set("a", "x"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("b", "y"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("a"); err != nil {
		t.Fatal(err)
	}
	got := load(t)
	if _, ok := got["a"]; ok || got.Get("b") != "y" {
		t.Fatalf("want only b left, got %v", got)
	}
}

func TestSaveIsAtomic(t *testing.T) {
	setup(t)
	if err := load(t).Set("a", "x"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(file()))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "notes.json" {
		t.Fatalf("want only notes.json in the state dir, got %v", entries)
	}
}

func TestCorruptFileIsReportedAndKept(t *testing.T) {
	setup(t)
	if err := os.MkdirAll(filepath.Dir(file()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file(), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load()
	if err == nil {
		t.Fatal("want an error for a corrupt notes.json")
	}
	if s == nil || len(s) != 0 {
		t.Fatalf("want an empty usable store, got %v", s)
	}
	if b, _ := os.ReadFile(file()); string(b) != "{not json" {
		t.Fatalf("Load changed the corrupt file: %q", b)
	}
}

func TestSaveOverCorruptFileKeepsBackup(t *testing.T) {
	setup(t)
	if err := os.MkdirAll(filepath.Dir(file()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file(), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, _ := Load()
	if err := s.Set("a", "x"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(file() + ".bak"); string(b) != "{not json" {
		t.Fatalf("want the corrupt file kept as notes.json.bak, got %q", b)
	}
	if load(t).Get("a") != "x" {
		t.Fatal("note not saved")
	}
}
