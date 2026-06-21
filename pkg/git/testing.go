package git

import (
	"context"
	"os/exec"
)

// ExecCommandFunc mirrors the signature of exec.CommandContext, allowing
// callers to substitute the subprocess execution mechanism used internally
// by RunGitCommand.
type ExecCommandFunc func(ctx context.Context, name string, args ...string) *exec.Cmd

// SetExecCommandForTest overrides the subprocess execution function used by
// RunGitCommand and returns a restore function that resets it back to the
// previous value. Intended exclusively for test mocking (e.g. routing git
// subprocess calls to a TestHelperProcess in the calling package), allowing
// packages outside pkg/git (such as cmd) to exercise RunGitCommand-based
// logic without touching the real filesystem or network.
func SetExecCommandForTest(fn ExecCommandFunc) (restore func()) {
	prev := execCommand
	execCommand = fn
	return func() { execCommand = prev }
}
