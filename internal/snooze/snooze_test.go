package snooze

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/chriha/lanes/internal/app"
)

// Thursday, 2026-10-29 15:04 in a fixed non-UTC zone.
var (
	zone = time.FixedZone("CET", 3600)
	now  = time.Date(2026, 10, 29, 15, 4, 0, 0, zone)
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, zone) }

func TestParse(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Time
		wantErr bool
	}{
		{"", time.Time{}, false},
		{"   ", time.Time{}, false},
		{"30m", now.Add(30 * time.Minute), false},
		{"2h", now.Add(2 * time.Hour), false},
		{"1h30m", now.Add(90 * time.Minute), false},
		{"3d", now.AddDate(0, 0, 3), false},
		{" 1D ", now.AddDate(0, 0, 1), false},
		{"mon", day(2026, 11, 2), false},
		{"Monday", day(2026, 11, 2), false},
		{"wed", day(2026, 11, 4), false},
		{"fri", day(2026, 10, 30), false},
		{"thu", day(2026, 11, 5), false}, // today is Thursday: next week's
		{"2026-11-03", day(2026, 11, 3), false},
		{"3x", time.Time{}, true},
		{"d", time.Time{}, true},
		{"0d", time.Time{}, true},
		{"-2h", time.Time{}, true},
		{"2026-10-29", time.Time{}, true}, // today's midnight is past
		{"2026-10-01", time.Time{}, true},
		{"2026-13-01", time.Time{}, true},
		{"mo", time.Time{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := Parse(tt.in, now)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !got.Equal(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAwake(t *testing.T) {
	tests := []struct {
		name string
		e    Entry
		at   time.Time
		fp   string
		want bool
	}{
		{"before the time", Entry{Until: now}, now.Add(-time.Second), "x", false},
		{"at the time", Entry{Until: now}, now, "", true},
		{"after the time", Entry{Until: now}, now.Add(time.Hour), "", true},
		{"timed snooze ignores changes", Entry{Until: now, Fingerprint: "a"}, now.Add(-time.Hour), "b", false},
		{"unchanged", Entry{Fingerprint: "a"}, now.AddDate(1, 0, 0), "a", false},
		{"changed", Entry{Fingerprint: "a"}, now, "b", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.e.Awake(tt.at, tt.fp); got != tt.want {
				t.Errorf("Awake = %v, want %v", got, tt.want)
			}
		})
	}
}

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	app.Init(t.TempDir())
}

func file() string { return filepath.Join(app.StateDir(), "snoozes.json") }

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
		t.Fatalf("want no snoozes, got %v", s)
	}
}

func TestSetDeleteSurviveReload(t *testing.T) {
	setup(t)
	s := load(t)
	timed, changed := Entry{Until: now}, Entry{Fingerprint: "abc"}
	if err := s.Set(timed, "!1", "!2"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(changed, "#3"); err != nil {
		t.Fatal(err)
	}
	got := load(t)
	if len(got) != 3 || !got["!1"].Until.Equal(now) || !got["!2"].Until.Equal(now) || got["#3"] != changed {
		t.Fatalf("after reload got %v", got)
	}
	if err := s.Delete("!1", "#3"); err != nil {
		t.Fatal(err)
	}
	if got := load(t); !reflect.DeepEqual(keys(got), []string{"!2"}) {
		t.Fatalf("want only !2 left, got %v", got)
	}
}

func keys(s Store) []string {
	var k []string
	for id := range s {
		k = append(k, id)
	}
	return k
}

func TestSaveIsAtomic(t *testing.T) {
	setup(t)
	if err := load(t).Set(Entry{Until: now}, "a"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(file()))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "snoozes.json" {
		t.Fatalf("want only snoozes.json in the state dir, got %v", entries)
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
		t.Fatal("want an error for a corrupt snoozes.json")
	}
	if s == nil || len(s) != 0 {
		t.Fatalf("want an empty usable store, got %v", s)
	}
	if b, _ := os.ReadFile(file()); string(b) != "{not json" {
		t.Fatalf("Load changed the corrupt file: %q", b)
	}
	if err := s.Set(Entry{Fingerprint: "x"}, "a"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(file() + ".bak"); string(b) != "{not json" {
		t.Fatalf("want the corrupt file kept as snoozes.json.bak, got %q", b)
	}
	if load(t)["a"].Fingerprint != "x" {
		t.Fatal("snooze not saved")
	}
}

func TestUnreadableFileIsNotBackedUp(t *testing.T) {
	setup(t)
	if err := load(t).Set(Entry{Fingerprint: "x"}, "a"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file(), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(file()); err == nil {
		t.Skip("file still readable (running as root)")
	}
	if err := (Store{"b": {Fingerprint: "y"}}).Set(Entry{Fingerprint: "z"}, "c"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file() + ".bak"); err == nil {
		t.Fatal("a read error moved snoozes.json aside as if it were corrupt")
	}
}

func TestSaveDirIsPrivate(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	app.Init(t.TempDir())
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set(Entry{}, "a"); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]os.FileMode{filepath.Dir(path()): 0o700, path(): 0o600} {
		if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s = %v, %v; want %o", p, fi, err, want)
		}
	}
}
