package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
)

// execCommand is a package-level variable that wraps exec.CommandContext,
// allowing tests to mock subprocess execution.
var execCommand = exec.CommandContext

// tokenRegex matches HTTPS credentials/tokens in git URLs.
var tokenRegex = regexp.MustCompile(`(https?://)([^@\s]+)@`)

// Sanitize strips potential access tokens or credentials from git URLs in terminal outputs.
func Sanitize(input string) string {
	return tokenRegex.ReplaceAllString(input, "${1}[REDACTED]@")
}

// ValidateDestination checks if a target directory exists and whether it represents
// a valid initialized Git repository (containing a .git folder).
// - Returns (false, nil) if the path does not exist (valid target for clone).
// - Returns (true, nil) if the path exists and contains a valid .git directory (skip clone/allow pull).
// - Returns (false, err) if the path exists but is a file or is a directory without a valid .git folder.
func ValidateDestination(path string) (bool, error) {
	if path == "" {
		return false, fmt.Errorf("destination path cannot be empty")
	}

	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	if !fi.IsDir() {
		return false, fmt.Errorf("destination path %q exists and is a file, not a directory", path)
	}

	gitDir := filepath.Join(path, ".git")
	gfi, err := os.Stat(gitDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("destination path %q exists but is not a valid Git repository (missing .git directory)", path)
		}
		return false, err
	}

	if !gfi.IsDir() {
		return false, fmt.Errorf("destination path %q exists but .git is not a directory", path)
	}

	return true, nil
}

// RunGitCommand executes the system `git` command with the provided arguments in the specified directory.
// It configures environment variables to bypass interactive prompts and uses context control to enforce timeouts.
// Returns the sanitized combined stdout/stderr output and any sanitized execution error.
func RunGitCommand(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := execCommand(ctx, "git", args...)
	cmd.Dir = dir

	if len(cmd.Env) == 0 {
		cmd.Env = os.Environ()
	}
	cmd.Env = append(cmd.Env,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_SSH_COMMAND=ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ControlMaster=no",
	)

	output, err := cmd.CombinedOutput()
	sanitizedOutput := Sanitize(string(output))

	if err != nil {
		// exec.Cmd's Wait error (e.g. "signal: killed") does not itself wrap
		// the context error, so it must be checked separately to make
		// errors.Is(err, context.Canceled/DeadlineExceeded) work for callers.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return []byte(sanitizedOutput), fmt.Errorf("git command failed: %s: %w", Sanitize(err.Error()), ctxErr)
		}
		return []byte(sanitizedOutput), fmt.Errorf("git command failed: %s: %w", Sanitize(err.Error()), err)
	}

	return []byte(sanitizedOutput), nil
}
