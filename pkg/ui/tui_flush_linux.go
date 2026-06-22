//go:build linux

package ui

import (
	"os"
	"time"

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
