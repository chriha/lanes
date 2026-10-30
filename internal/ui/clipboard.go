package ui

import (
	"errors"
	"io"
)

var goosName = ""

// copyText is a stub.
func copyText(goos string, look func(string) (string, error), run func(path string, args []string, text string) error, osc io.Writer, text string) error {
	return errors.New("not implemented")
}

func systemCopy(text string) error { return errors.New("not implemented") }

func (m *Model) copyIDs() {}
