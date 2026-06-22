//go:build darwin

package ui

import (
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// flushRead is BSD's <fcntl.h> FREAD flag, used as TIOCFLUSH's queue
// selector. golang.org/x/sys/unix exports TIOCFLUSH for darwin but not
// this flag, so it is defined locally.
const flushRead = 0x1

// FlushStdin discards any unread data in the stdin buffer, particularly
// to clear terminal response sequences that might have arrived late.
func FlushStdin() {
	// Allow a brief pause (e.g., 10ms) for any pending terminal responses to arrive in the buffer.
	time.Sleep(10 * time.Millisecond)
	fd := int(os.Stdin.Fd())
	_ = unix.IoctlSetInt(fd, unix.TIOCFLUSH, flushRead)
}
