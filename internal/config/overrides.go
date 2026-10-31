package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chriha/lanes/internal/app"
)

// Overrides are edits made inside the TUI (:filter, :newview). They live in the state dir so the
// hand-written config stays untouched; a view's override wins over its config filter.
type Overrides struct {
	Filters map[string]string `json:"filters"`
	Views   []View            `json:"views"`

	backup string // set by Save when it moved a corrupt views.json aside
}

func overridesPath() string { return filepath.Join(app.StateDir(), "views.json") }

// LoadOverrides returns the saved overrides. A missing file is empty overrides; a corrupt one yields
// empty overrides and an error, so callers can report it.
func LoadOverrides() (*Overrides, error) {
	o := &Overrides{Filters: map[string]string{}}
	var err error
	if b, rerr := os.ReadFile(overridesPath()); rerr == nil {
		if err = json.Unmarshal(b, o); err != nil {
			err = fmt.Errorf("%s: %w", overridesPath(), err)
			o = &Overrides{}
		}
	}
	if o.Filters == nil {
		o.Filters = map[string]string{}
	}
	return o, err
}

// BackedUp is the path a corrupt views.json was moved to by the last Save, or "" if none was.
func (o *Overrides) BackedUp() string { return o.backup }

// Save writes the overrides. A views.json that reads but doesn't decode is moved to views.json.bak
// first, so the save never discards its content.
func (o *Overrides) Save() error {
	if err := os.MkdirAll(filepath.Dir(overridesPath()), app.DirMode); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(o, "", "  ")
	return app.WriteFile(overridesPath(), b)
}
