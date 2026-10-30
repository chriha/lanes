// Package snooze keeps the items the user put aside, until a time or until they change, in one JSON file in
// the project state dir.
package snooze

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chriha/lanes/internal/app"
)

// Entry is one snooze: until a time when Until is set, otherwise until the item's fingerprint differs.
type Entry struct {
	Until       time.Time `json:"until"`
	Fingerprint string    `json:"fingerprint,omitempty"`
}

// Awake reports whether the snooze is over: its time has come, or the item's current fingerprint differs.
func (e Entry) Awake(now time.Time, fingerprint string) bool {
	if !e.Until.IsZero() {
		return !now.Before(e.Until)
	}
	return fingerprint != e.Fingerprint
}

// Parse turns the snooze prompt's input into a wake time: a duration (`30m`, `2h`, `3d`), a weekday (`mon`,
// `monday`; its next occurrence, never today) or a date (`2026-11-03`), each relative to now in now's location.
// Empty input returns the zero time: until the item changes. A time that is not in the future is an error.
func Parse(input string, now time.Time) (time.Time, error) {
	s := strings.ToLower(strings.TrimSpace(input))
	if s == "" {
		return time.Time{}, nil
	}
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var t time.Time
	days, isDays := strings.CutSuffix(s, "d")
	n, daysErr := strconv.Atoi(days)
	if wd, ok := weekday(s); ok { // before days: "wed" ends in d
		ahead := (int(wd)-int(now.Weekday())+6)%7 + 1 // 1..7: the next one, a week out when it is today
		t = midnight.AddDate(0, 0, ahead)
	} else if isDays && daysErr == nil {
		t = now.AddDate(0, 0, n)
	} else if d, err := time.ParseDuration(s); err == nil {
		t = now.Add(d)
	} else if d, err := time.ParseInLocation(time.DateOnly, s, now.Location()); err == nil {
		t = d
	} else {
		return time.Time{}, fmt.Errorf("snooze %q: want a duration like 2h or 3d, a weekday or a date", input)
	}
	if !t.After(now) {
		return time.Time{}, fmt.Errorf("snooze %q: %s is not in the future", input, t.Format(time.DateTime))
	}
	return t, nil
}

func weekday(s string) (time.Weekday, bool) {
	for d := time.Sunday; d <= time.Saturday; d++ {
		name := strings.ToLower(d.String())
		if s == name || s == name[:3] {
			return d, true
		}
	}
	return 0, false
}

// Store maps item IDs to their snoozes.
type Store map[string]Entry

func path() string { return filepath.Join(app.StateDir(), "snoozes.json") }

// Load returns the saved snoozes. A missing file is an empty store; a corrupt one yields an empty store and an
// error, and stays on disk until the next Set or Delete replaces it.
func Load() (Store, error) {
	s := Store{}
	b, err := os.ReadFile(path())
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err == nil {
		err = json.Unmarshal(b, &s)
	}
	if err != nil {
		return Store{}, fmt.Errorf("%s: %w", path(), err)
	}
	return s, nil
}

// Set snoozes the items and writes the file.
func (s Store) Set(e Entry, ids ...string) error {
	for _, id := range ids {
		s[id] = e
	}
	return s.save()
}

// Delete unsnoozes the items (woken, expired or by hand) and writes the file.
func (s Store) Delete(ids ...string) error {
	for _, id := range ids {
		delete(s, id)
	}
	return s.save()
}

// save writes through a temp file and a rename, so a crash never leaves a half-written snoozes.json.
// A corrupt snoozes.json is moved to snoozes.json.bak first, so the first save after it is not lossy.
// ponytail: copy of notes.save; extract a shared atomic JSON writer if a third store appears.
// ponytail: last writer wins across lanes instances (same as notes).
func (s Store) save() error {
	dir := filepath.Dir(path())
	if err := os.MkdirAll(dir, app.DirMode); err != nil {
		return err
	}
	// Only a file that reads but doesn't decode is corrupt; a read error must not move a healthy file aside.
	if b, err := os.ReadFile(path()); err == nil && json.Unmarshal(b, &Store{}) != nil {
		if err := os.Rename(path(), path()+".bak"); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".snoozes-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name()) // no-op after a successful rename
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path())
}
