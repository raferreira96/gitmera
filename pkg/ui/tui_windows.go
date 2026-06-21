//go:build windows

package ui

// FlushStdin is a no-op on Windows.
func FlushStdin() {
}

// DrainStdinNonblocking is a no-op on Windows.
func DrainStdinNonblocking() {
}
