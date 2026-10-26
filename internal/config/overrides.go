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

func (o *Overrides) Save() error {
	if err := os.MkdirAll(filepath.Dir(overridesPath()), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(o, "", "  ")
	return os.WriteFile(overridesPath(), b, 0o644)
}
