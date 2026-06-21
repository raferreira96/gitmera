//go:build !windows

package ui

import (
	"os"
	"time"

	"github.com/mattn/go-isatty"
	"golang.org/x/sys/unix"
)

// FlushStdin discards any unread data in the stdin buffer, particularly
// to clear terminal response sequences that might have arrived late.
func FlushStdin() {
	// Allow a brief pause (e.g., 10ms) for any pending terminal responses to arrive in the buffer.
	time.Sleep(10 * time.Millisecond)
	fd := int(os.Stdin.Fd())
	_ = unix.IoctlSetInt(fd, unix.TCFLSH, unix.TCIFLUSH)
}

// DrainStdinNonblocking reads and discards all currently available data on stdin
// without blocking, to clear orphaned terminal response sequences.
func DrainStdinNonblocking() {
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return
	}
	fd := int(os.Stdin.Fd())

	savedFlags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if err != nil {
		return
	}

	err = unix.SetNonblock(fd, true)
	if err != nil {
		return
	}

	defer func() {
		_, _ = unix.FcntlInt(uintptr(fd), unix.F_SETFL, savedFlags)
	}()

	buf := make([]byte, 64)
	for {
		n, err := unix.Read(fd, buf)
		if err != nil {
			if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
				break
			}
			break
		}
		if n == 0 {
			break
		}
	}
}
