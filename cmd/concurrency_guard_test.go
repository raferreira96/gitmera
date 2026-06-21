package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// concurrencyGuardConfig is a minimal, syntactically valid .gitmera.yaml
// document: one project with a Git URI matching pkg/config's gitURIRegex
// and a relative path, sufficient for config.Load to succeed so the
// concurrency guard (not an earlier config error) is what gets exercised.
const concurrencyGuardConfig = `version: "1"
projects:
  example:
    repo: "git@github.com:example/api.git"
    path: "./api"
`

// writeConcurrencyGuardConfig creates a minimal valid .gitmera.yaml in a
// fresh temp directory and returns its path.
func writeConcurrencyGuardConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitmera.yaml")
	if err := os.WriteFile(path, []byte(concurrencyGuardConfig), 0o644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}

// concurrencyGuardCommands lists the five commands under test, along with
// the positional args each one requires (checkout requires exactly one
// branch argument per cobra.ExactArgs(1); the others take none).
var concurrencyGuardCommands = []struct {
	name string
	cmd  *cobra.Command
	args []string
}{
	{name: "status", cmd: statusCmd, args: []string{}},
	{name: "checkout", cmd: checkoutCmd, args: []string{"test-branch"}},
	{name: "push", cmd: pushCmd, args: []string{}},
	{name: "pull", cmd: pullCmd, args: []string{}},
	{name: "clone", cmd: cloneCmd, args: []string{}},
}

// withConcurrencyGuardConfig points the package-level cfgFile var (which
// backs the persistent --config/-c flag read by resolveConfigPath in every
// command's RunE) at a minimal valid config for the duration of the test,
// restoring the previous value afterward.
func withConcurrencyGuardConfig(t *testing.T) {
	t.Helper()
	prev := cfgFile
	cfgFile = writeConcurrencyGuardConfig(t)
	t.Cleanup(func() { cfgFile = prev })
}

// TestConcurrencyGuard_RejectsZeroAndNegative proves all five commands
// (status, checkout, push, pull, clone) reject concurrency values below 1
// immediately, before runner.ExecuteTasks is ever reached. The temp
// directory's config references a repo path ("./api") that does not exist
// as a real Git repository, so if the guard were missing or bypassed,
// ExecuteTasks would either hang (the original CR-01 defect, in which case
// this test would time out rather than pass) or fail for an unrelated
// reason (a different error message, also causing the assertion below to
// fail) — never silently pass for the wrong reason.
func TestConcurrencyGuard_RejectsZeroAndNegative(t *testing.T) {
	for _, tc := range concurrencyGuardCommands {
		for _, badValue := range []string{"0", "-1"} {
			tc, badValue := tc, badValue
			t.Run(tc.name+"/-j="+badValue, func(t *testing.T) {
				withConcurrencyGuardConfig(t)

				if err := tc.cmd.Flags().Set("concurrency", badValue); err != nil {
					t.Fatalf("failed to set concurrency flag: %v", err)
				}
				t.Cleanup(func() {
					_ = tc.cmd.Flags().Set("concurrency", "5")
				})

				tc.cmd.SetContext(context.Background())
				err := tc.cmd.RunE(tc.cmd, tc.args)
				if err == nil {
					t.Fatalf("expected an error for -j %s, got nil", badValue)
				}
				if !strings.Contains(err.Error(), "concurrency must be a positive integer") {
					t.Fatalf("expected concurrency guard error, got: %v", err)
				}
			})
		}
	}
}

// TestConcurrencyGuard_AllowsValidValues confirms valid concurrency values
// (1, 5, and the flag left unset/default) all proceed past the guard
// without triggering the concurrency error. These commands perform real git
// operations against a nonexistent repo path in the temp dir, so a later,
// unrelated error (e.g. a failed git invocation) is expected and ignored;
// only the absence of the concurrency message is asserted.
func TestConcurrencyGuard_AllowsValidValues(t *testing.T) {
	for _, tc := range concurrencyGuardCommands {
		for _, validValue := range []string{"1", "5", ""} {
			tc, validValue := tc, validValue
			label := "default"
			if validValue != "" {
				label = "-j=" + validValue
			}
			t.Run(tc.name+"/"+label, func(t *testing.T) {
				withConcurrencyGuardConfig(t)

				if validValue != "" {
					if err := tc.cmd.Flags().Set("concurrency", validValue); err != nil {
						t.Fatalf("failed to set concurrency flag: %v", err)
					}
				}
				t.Cleanup(func() {
					_ = tc.cmd.Flags().Set("concurrency", "5")
				})

				tc.cmd.SetContext(context.Background())
				err := tc.cmd.RunE(tc.cmd, tc.args)
				if err != nil && strings.Contains(err.Error(), "concurrency must be a positive integer") {
					t.Fatalf("guard incorrectly fired for valid value %q: %v", label, err)
				}
			})
		}
	}
}
