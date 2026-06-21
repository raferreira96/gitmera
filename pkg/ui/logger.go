// Package ui provides thread-safe, TTY-aware terminal output primitives for
// Gitmera's CLI commands. All concurrent writes to stdout/stderr must be
// routed through SafeLogger to avoid scrambled or interleaved terminal
// output when multiple goroutines report progress simultaneously.
package ui

import (
	"fmt"
	"io"
	"os"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/mattn/go-isatty"
)

// SafeLogger serializes all writes to an underlying io.Writer behind a
// sync.Mutex, preventing character/line interleaving when multiple
// goroutines print concurrently. It automatically downgrades (or disables)
// ANSI color/style output when the target stream is not a terminal, or when
// forceNoColor is requested explicitly (e.g. via --no-color).
type SafeLogger struct {
	mu      sync.Mutex
	out     io.Writer
	cpw     *colorprofile.Writer
	noColor bool
}

// NewSafeLogger constructs a SafeLogger writing to out. It uses go-isatty to
// detect whether out is an interactive terminal; if it is not (e.g. output
// is redirected to a file or pipe, or running in CI), or if forceNoColor is
// true, ANSI escape sequences are stripped entirely from all rendered
// output (NoTTY profile) rather than merely downgraded, guaranteeing clean
// plain-text output for redirected/non-interactive streams.
func NewSafeLogger(out io.Writer, forceNoColor bool) *SafeLogger {
	isTerminal := false
	if f, ok := out.(*os.File); ok {
		isTerminal = isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
	}

	profile := colorprofile.Detect(out, os.Environ())
	if forceNoColor || !isTerminal {
		profile = colorprofile.NoTTY
	}

	cpw := &colorprofile.Writer{
		Forward: out,
		Profile: profile,
	}

	return &SafeLogger{
		out:     out,
		cpw:     cpw,
		noColor: forceNoColor || !isTerminal,
	}
}

// IsColorEnabled reports whether this logger renders ANSI color/style
// sequences (true) or downgrades to plain ASCII text (false).
func (l *SafeLogger) IsColorEnabled() bool {
	return !l.noColor
}

// IsInteractiveTerminal reports whether out is an interactive terminal
// (TTY), independent of any --no-color preference. Callers use this to
// decide whether to launch the Bubble Tea TUI or fall back to sequential
// logging (D-05, D-07): the TUI must never be started when stdout is
// redirected to a file, pipe, or CI log collector.
func IsInteractiveTerminal(out io.Writer) bool {
	f, ok := out.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// LogSuccess prints a styled checkmark followed by a bold repository name
// and a status message, e.g. "✓ api: success". The write is guarded by the
// internal mutex and color-downsampled automatically for non-TTY targets.
func (l *SafeLogger) LogSuccess(repoName, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	checkmark := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("✓")
	repoStyle := lipgloss.NewStyle().Bold(true).Render(repoName)

	fmt.Fprintf(l.cpw, "%s %s: %s\n", checkmark, repoStyle, message)
}

// LogErrorBox prints a red-bordered error box detailing a failed operation:
// a bold red header naming the failed repository, the error, and the full
// captured stderr output. The write is guarded by the internal mutex and
// color-downsampled automatically for non-TTY targets.
func (l *SafeLogger) LogErrorBox(repoName string, err error, stderr string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("9")).
		Bold(true)

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("9")).
		Padding(0, 1)

	header := headerStyle.Render(fmt.Sprintf("FAILURE: %s", repoName))
	body := fmt.Sprintf("Error: %v\n\n%s", err, stderr)

	content := fmt.Sprintf("%s\n%s", header, body)
	box := borderStyle.Render(content)

	fmt.Fprintln(l.cpw, box)
}

// Print writes a raw message to the underlying writer under the mutex lock,
// without any additional styling. Useful for plain status/abort messages.
func (l *SafeLogger) Print(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprint(l.cpw, msg)
}
