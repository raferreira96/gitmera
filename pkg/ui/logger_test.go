package ui_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"gitmera/pkg/ui"
)

// TestNewSafeLogger_NonTTYDisablesColor verifies that constructing a
// SafeLogger against a non-terminal io.Writer (e.g. a bytes.Buffer, which is
// never an *os.File and thus never detected as a TTY) automatically
// downgrades styling, leaving no raw ANSI escape sequences in the output.
func TestNewSafeLogger_NonTTYDisablesColor(t *testing.T) {
	var buf bytes.Buffer
	logger := ui.NewSafeLogger(&buf, false)

	if logger.IsColorEnabled() {
		t.Error("expected IsColorEnabled() to be false for a non-TTY writer")
	}

	logger.LogSuccess("api", "success")

	out := buf.String()
	if strings.Contains(out, "\x1b[") {
		t.Errorf("expected no ANSI escape sequences in non-TTY output, got: %q", out)
	}
	if !strings.Contains(out, "api") || !strings.Contains(out, "success") {
		t.Errorf("expected output to contain repo name and message, got: %q", out)
	}
}

// TestNewSafeLogger_ForceNoColor verifies that forceNoColor strips styling
// even if (hypothetically) the writer were detected as a terminal.
func TestNewSafeLogger_ForceNoColor(t *testing.T) {
	var buf bytes.Buffer
	logger := ui.NewSafeLogger(&buf, true)

	if logger.IsColorEnabled() {
		t.Error("expected IsColorEnabled() to be false when forceNoColor is true")
	}

	logger.Print("plain message")
	if got := buf.String(); got != "plain message" {
		t.Errorf("expected raw message to pass through unchanged, got: %q", got)
	}
}

// TestLogErrorBox_ContainsFailureDetails verifies the error box includes the
// repository name, the error text, and the captured stderr output.
func TestLogErrorBox_ContainsFailureDetails(t *testing.T) {
	var buf bytes.Buffer
	logger := ui.NewSafeLogger(&buf, true)

	logger.LogErrorBox("web", errors.New("exit status 1"), "fatal: not a git repository")

	out := buf.String()
	for _, want := range []string{"web", "exit status 1", "fatal: not a git repository"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected error box output to contain %q, got: %q", want, out)
		}
	}
}

// TestIsInteractiveTerminal_NonFile verifies that a non-*os.File writer (such
// as bytes.Buffer) always returns false, since it can never be a TTY.
func TestIsInteractiveTerminal_NonFile(t *testing.T) {
	var buf bytes.Buffer
	if ui.IsInteractiveTerminal(&buf) {
		t.Error("expected IsInteractiveTerminal=false for a non-*os.File writer")
	}
}

// TestIsInteractiveTerminal_File exercises the *os.File branch. os.Stderr is a
// real file descriptor but is not a TTY in a test process, so it must return false.
func TestIsInteractiveTerminal_File(t *testing.T) {
	if ui.IsInteractiveTerminal(os.Stderr) {
		t.Error("expected IsInteractiveTerminal=false for os.Stderr in a non-TTY test process")
	}
}

// TestNewSafeLogger_OsFileWriter exercises the isatty detection branch inside
// NewSafeLogger when the writer is an *os.File. os.Stderr in a test process is
// not a TTY, so color must still be disabled.
func TestNewSafeLogger_OsFileWriter(t *testing.T) {
	logger := ui.NewSafeLogger(os.Stderr, false)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	if logger.IsColorEnabled() {
		t.Error("expected IsColorEnabled=false for os.Stderr in a non-TTY test process")
	}
}

// TestSafeLogger_ConcurrentWritesDoNotInterleave exercises the mutex
// guarantee: many goroutines calling Print/LogSuccess concurrently against a
// shared buffer must never produce interleaved/corrupted lines. Each
// goroutine writes a single, recognizable, complete line; we verify after
// the fact that every expected line appears intact and the total line count
// matches (no partial/merged lines).
func TestSafeLogger_ConcurrentWritesDoNotInterleave(t *testing.T) {
	var buf bytes.Buffer
	logger := ui.NewSafeLogger(&buf, true)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			logger.LogSuccess(fmt.Sprintf("repo%d", n), "success")
		}(i)
	}
	wg.Wait()

	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != goroutines {
		t.Fatalf("expected %d output lines, got %d; output: %q", goroutines, len(lines), out)
	}

	seen := make(map[string]bool, goroutines)
	for _, line := range lines {
		if !strings.Contains(line, ": success") {
			t.Errorf("line is corrupted/interleaved (missing expected suffix): %q", line)
			continue
		}
		seen[line] = true
	}

	for i := 0; i < goroutines; i++ {
		repo := fmt.Sprintf("repo%d", i)
		found := false
		for line := range seen {
			if strings.Contains(line, repo) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected a line for %q, not found in output", repo)
		}
	}
}
