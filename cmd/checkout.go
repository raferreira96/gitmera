package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gitmera/pkg/git"
	"gitmera/pkg/runner"
	"gitmera/pkg/ui"

	"github.com/spf13/cobra"
)

var (
	checkoutConcurrency int
	checkoutTimeout     time.Duration
	checkoutCreate      bool
)

var checkoutCmd = &cobra.Command{
	Use:          "checkout <branch>",
	Short:        "Switch or create a branch concurrently across all configured repositories",
	Long:         `Reads the workspace configuration and concurrently switches every child repository to the given branch, optionally creating it with -b. Repositories with uncommitted local changes are safely skipped (Safe Fail) without aborting the rest of the run.`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		branch := args[0]
		logger := ui.NewSafeLogger(cmd.OutOrStdout(), noColor)

		setup, err := setupCommand(cmd, checkoutConcurrency, checkoutTimeout, "checkout")
		if err != nil {
			return err
		}
		defer setup.cancel()

		action := func(workerCtx context.Context, task runner.RepoTask) (error, string, bool) {
			return checkoutRepo(workerCtx, task.Path, branch, checkoutCreate)
		}

		// Checkout always uses the keep-going policy (D-05): a repository
		// with uncommitted local changes (or any other failure) must be
		// safely skipped without aborting checkout on the remaining repos.
		interactive := isInteractiveMode(cmd.OutOrStdout())
		results := ui.OrchestrateExecution(setup.ctx, setup.tasks, setup.concurrency, false, setup.timeout, action, ui.ExecutionOptions{
			Interactive: interactive,
			ActionLabel: "checking out " + branch,
			Logger:      logger,
		})

		// The TUI/fallback already printed a generic status line per
		// repository (success, skipped, cancelled) as events were
		// dispatched. Here, at the end of the run, we additionally surface
		// the detailed reason for safe-skipped repositories (e.g. Safe
		// Switch fallback message) and the full error boxes for genuine
		// failures (D-13).
		if reportResults(results, logger) || setup.ctx.Err() != nil {
			return fmt.Errorf("checkout failed: one or more repositories failed to checkout %q", branch)
		}

		return nil
	},
}

// checkoutRepo performs the Safe Fail and Safe Switch checkout logic for a
// single repository: it first verifies the repository exists, then aborts
// for repos with uncommitted tracked changes (D-05), then either switches to
// an existing branch or creates a new one, safely falling back to a switch
// if -b targets a branch that already exists locally (D-06, D-07). No
// upstream tracking is configured here (D-08); that is deferred to the
// first push.
func checkoutRepo(ctx context.Context, path, branch string, create bool) (err error, warning string, skipped bool) {
	valid, verr := git.ValidateDestination(path)
	if verr != nil || !valid {
		if verr != nil {
			return verr, "", false
		}
		return fmt.Errorf("destination path %q does not exist or is not a valid Git repository", path), "", false
	}

	// Safe Fail (D-05): abort checkout for this repository if there are any
	// tracked modifications, deletions, or staged changes. Untracked files
	// (`??`) alone do not block checkout, since they cannot be overwritten
	// by switching branches.
	dirty, derr := hasUncommittedTrackedChanges(ctx, path)
	if derr != nil {
		return derr, "", false
	}
	if dirty {
		return fmt.Errorf("local changes would be overwritten by checkout"), "", false
	}

	if !create {
		// Plain switch to an existing branch (D-07).
		output, cerr := git.RunGitCommand(ctx, path, "checkout", branch)
		if cerr != nil {
			return cerr, string(output), false
		}
		return nil, "", false
	}

	// -b flag set: check whether the branch already exists locally first,
	// to implement the Safe Switch fallback (D-06).
	exists, eerr := localBranchExists(ctx, path, branch)
	if eerr != nil {
		return eerr, "", false
	}

	if exists {
		// Safe Switch: fall back to a plain checkout instead of failing
		// with "fatal: a branch named ... already exists". Reported as
		// skipped=true (despite err==nil) so the TUI/fallback renders this
		// as a yellow warning line rather than a duplicate green success
		// checkmark, consistent with D-16's "safe warning" semantics.
		output, cerr := git.RunGitCommand(ctx, path, "checkout", branch)
		if cerr != nil {
			return cerr, string(output), false
		}
		return nil, fmt.Sprintf("branch %q already exists, switched to it safely", branch), true
	}

	output, cerr := git.RunGitCommand(ctx, path, "checkout", "-b", branch)
	if cerr != nil {
		return cerr, string(output), false
	}
	return nil, "", false
}

// hasUncommittedTrackedChanges runs `git status --porcelain=v1` and reports
// whether any line represents a tracked modification, deletion, or staged
// change (i.e. any line that does not start with the untracked-file marker
// `??`). Untracked files alone are not considered blocking for checkout.
func hasUncommittedTrackedChanges(ctx context.Context, path string) (bool, error) {
	output, err := git.RunGitCommand(ctx, path, "status", "--porcelain=v1")
	if err != nil {
		return false, err
	}

	trimmed := strings.TrimRight(string(output), "\n")
	if trimmed == "" {
		return false, nil
	}

	for _, line := range strings.Split(trimmed, "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "??") {
			return true, nil
		}
	}
	return false, nil
}

// localBranchExists checks whether a local branch ref already exists using
// `git show-ref --verify --quiet refs/heads/<branch>`, scoping the lookup to
// refs/heads to avoid name collisions with tags or remote-tracking branches.
func localBranchExists(ctx context.Context, path, branch string) (bool, error) {
	_, err := git.RunGitCommand(ctx, path, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if err == nil {
		return true, nil
	}
	// show-ref exits non-zero (typically 1) when the ref does not exist;
	// this is an expected, non-fatal outcome, not an execution error.
	return false, nil
}

func init() {
	checkoutCmd.Flags().IntVarP(&checkoutConcurrency, "concurrency", "j", 5, "Maximum number of concurrent checkout operations")
	checkoutCmd.Flags().DurationVar(&checkoutTimeout, "timeout", 2*time.Minute, "Timeout for each individual checkout operation")
	checkoutCmd.Flags().BoolVarP(&checkoutCreate, "branch", "b", false, "Create and switch to a new branch (falls back to a safe switch if it already exists)")
	rootCmd.AddCommand(checkoutCmd)
}
