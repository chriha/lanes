//go:build unix

package termbg

import (
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// readByte reads one byte from f, waiting at most wait.
func readByte(f *os.File, wait time.Duration) (byte, bool) {
	if wait <= 0 {
		return 0, false
	}
	fd := int(f.Fd())
	var set unix.FdSet
	set.Set(fd)
	tv := unix.NsecToTimeval(int64(wait))
	for {
		n, err := unix.Select(fd+1, &set, nil, nil, &tv)
		if err == unix.EINTR {
			set = unix.FdSet{}
			set.Set(fd)
			continue
		}
		if err != nil || n == 0 {
			return 0, false
		}
		break
	}
	var b [1]byte
	if n, err := f.Read(b[:]); err != nil || n == 0 {
		return 0, false
	}
	return b[0], true
}

// foreground reports whether this process owns the terminal; a background process must not touch it.
func foreground(fd uintptr) bool {
	pgrp, err := unix.IoctlGetInt(int(fd), unix.TIOCGPGRP)
	return err == nil && pgrp == unix.Getpgrp()
}
