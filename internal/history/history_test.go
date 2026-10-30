package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chriha/lanes/internal/app"
)

func setup(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	app.Init(t.TempDir())
}

func TestForMissingFile(t *testing.T) {
	setup(t)
	if got := For("a"); len(got) != 0 {
		t.Fatalf("want no events, got %v", got)
	}
}

func TestAppendFor(t *testing.T) {
	setup(t)
	t0 := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	Append(Event{Time: t0, Item: "a", Action: "review", Event: "started"})
	Append(Event{Time: t0, Item: "b", Action: "review", Event: "started"})
	Append(Event{Time: t0.Add(time.Minute), Item: "a", Action: "review", Event: "failed", Exit: 2})

	tests := []struct {
		item   string
		events []string
	}{
		{"a", []string{"started", "failed"}},
		{"b", []string{"started"}},
		{"c", nil},
	}
	for _, tt := range tests {
		got := For(tt.item)
		if len(got) != len(tt.events) {
			t.Fatalf("For(%q): got %d events, want %d", tt.item, len(got), len(tt.events))
		}
		for i, e := range got {
			if e.Event != tt.events[i] || e.Item != tt.item {
				t.Errorf("For(%q)[%d] = %+v, want event %s", tt.item, i, e, tt.events[i])
			}
		}
	}
	if got := For("a")[1]; got.Exit != 2 || !got.Time.Equal(t0.Add(time.Minute)) {
		t.Errorf("round trip lost fields: %+v", got)
	}
}

func TestAppendSetsTime(t *testing.T) {
	setup(t)
	Append(Event{Item: "a", Event: "started"})
	if got := For("a"); len(got) != 1 || got[0].Time.IsZero() {
		t.Fatalf("want one event with a time, got %+v", got)
	}
}

func TestForSkipsCorruptLines(t *testing.T) {
	setup(t)
	Append(Event{Item: "a", Event: "started"})
	f, err := os.OpenFile(path(), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("not json\n{\"item\":\n\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	Append(Event{Item: "a", Event: "finished"})

	got := For("a")
	if len(got) != 2 || got[0].Event != "started" || got[1].Event != "finished" {
		t.Fatalf("want started,finished around corrupt lines, got %+v", got)
	}
}

func TestPathUnderTempState(t *testing.T) {
	setup(t)
	if rel, err := filepath.Rel(os.Getenv("XDG_STATE_HOME"), path()); err != nil || rel[0] == '.' {
		t.Fatalf("history path %s escapes state dir", path())
	}
}

func TestAppendIsPrivate(t *testing.T) {
	setup(t)
	if err := os.MkdirAll(filepath.Dir(path()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path(), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	Append(Event{Item: "a"})
	if fi, err := os.Stat(path()); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("existing file = %v, %v; want 0600", fi, err)
	}
	setup(t)
	Append(Event{Item: "a"})
	fi, err := os.Stat(path())
	di, derr := os.Stat(filepath.Dir(path()))
	if err != nil || derr != nil || fi.Mode().Perm() != 0o600 || di.Mode().Perm() != 0o700 {
		t.Errorf("new file/dir = %v %v, %v %v; want 0600/0700", fi, err, di, derr)
	}
}
