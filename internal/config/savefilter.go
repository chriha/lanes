package config

import "errors"

// ErrNotSavable means SaveViewFilter could not place the filter safely and left the file unchanged.
var ErrNotSavable = errors.New("filter can't be saved")

// ViewFile returns the config file whose [[view]] named name is in effect.
func ViewFile(name string) (string, error) { return "", errors.New("not implemented") }

// SaveViewFilter writes expr as the filter of the [[view]] named view in the TOML file at path.
func SaveViewFilter(path, view, expr string) error { return errors.New("not implemented") }
