// Package jsonfile writes the JSON files of lanes' small stores (notes, snoozes, seen, trust) atomically.
package jsonfile

import "errors"

// Write is not implemented yet.
func Write[T any](path string, v T, backupCorrupt bool) error {
	return errors.New("not implemented")
}
