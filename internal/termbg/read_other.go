//go:build !unix

package termbg

import (
	"os"
	"time"
)

// readByte never reads: without select on a terminal the query is skipped rather than risk a blocked read.
func readByte(*os.File, time.Duration) (byte, bool) { return 0, false }

// foreground is false, so query never talks to the terminal.
func foreground(uintptr) bool { return false }
