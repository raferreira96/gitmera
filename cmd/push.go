package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gitmera/pkg/git"
	"gitmera/pkg/runner"
	"gitmera/pkg/ui"

	"github.com/spf13/cobra"
)

var (
	pushConcurrency int
	pushTimeout     time.Duration
)

var pushCmd = &cobra.Command{
	Use:          "push",
	Short:        "Smart-push the current branch concurrently across all configured repositories",
	Long:         `Reads the workspace configuration and concurrently pushes the current branch to origin in every child repository, skipping repositories that are already up-to-date or behind/diverged, and auto-configuring upstream tracking for branches that don't have it yet.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := ui.NewSafeLogger(cmd.OutOrStdout(), noColor)

		setup, err := setupCommand(cmd, pushConcurrency, pushTimeout, "push")
		if err != nil {
			return err
		}
		defer setup.cancel()

		action := func(workerCtx context.Context, task runner.RepoTask) (string, bool, error) {
			return pushRepo(workerCtx, task.Path)
		}

		// Push always uses the keep-going policy: repositories that are
		// behind/diverged or otherwise fail to push must not abort the run
		// for the remaining repositories (D-11).
		interactive := isInteractiveMode(cmd.OutOrStdout())
		results := ui.OrchestrateExecution(setup.ctx, setup.tasks, setup.concurrency, false, setup.timeout, action, ui.ExecutionOptions{
			Interactive: interactive,
			ActionLabel: "pushing",
			Logger:      logger,
		})

		// The TUI/fallback already printed a generic status line per
		// repository (success, skipped, cancelled) as events were
		// dispatched. Here, at the end of the run, we additionally surface
		// the detailed reason for safe-skipped repositories (already
		// up-to-date, or Safe Abort due to behind/diverged remote) and the
		// full error boxes for genuine failures (D-13, D-16).
		if reportResults(results, logger) || setup.ctx.Err() != nil {
			return fmt.Errorf("push failed: one or more repositories failed to push")
		}

		return nil
	},
}

// pushRepo performs the Smart Push pre-check and execution logic for a
// single repository: it determines the current branch, checks whether an
// upstream is configured, auto-sets the upstream on first push (D-10,
// D-12), or otherwise computes the local ahead/behind divergence and either
// skips (in-sync), Safe Aborts (behind/diverged, D-11), or pushes (ahead).
func pushRepo(ctx context.Context, path string) (reason string, skipped bool, err error) {
	alreadyCloned, verr := git.ValidateDestination(path)
	if verr != nil || !alreadyCloned {
		if verr != nil {
			return "", false, verr
		}
		return "", false, fmt.Errorf("destination path %q does not exist or is not a valid Git repository", path)
	}

	branch, berr := currentBranch(ctx, path)
	if berr != nil {
		return "", false, berr
	}

	hasUpstream, uerr := hasUpstreamConfigured(ctx, path)
	if uerr != nil {
		return "", false, uerr
	}

	if !hasUpstream {
		// D-10/D-12: no tracking branch configured yet — push and set the
		// upstream to origin automatically on this first push.
		output, perr := git.RunGitCommand(ctx, path, "push", "--set-upstream", "origin", branch)
		if perr != nil {
			return string(output), false, perr
		}
		return "", false, nil
	}

	ahead, behind, derr := aheadBehindCount(ctx, path)
	if derr != nil {
		return "", false, derr
	}

	if behind > 0 {
		// D-11: Safe Abort. Never force-push; skip with a detailed warning
		// whether behind only or fully diverged (ahead > 0 && behind > 0).
		if ahead > 0 {
			return fmt.Sprintf("skipped: diverged from remote (ahead %d, behind %d) — manual merge/rebase required", ahead, behind), true, nil
		}
		return fmt.Sprintf("skipped: behind remote by %d commit(s) — pull or rebase before pushing", behind), true, nil
	}

	if ahead == 0 {
		// Already up-to-date: skip to avoid an unnecessary network call.
		return "skipped: already up-to-date with remote", true, nil
	}

	output, perr := git.RunGitCommand(ctx, path, "push", "origin", branch)
	if perr != nil {
		return string(output), false, perr
	}
	return "", false, nil
}

// currentBranch resolves the name of the currently checked-out branch using
// `git rev-parse --abbrev-ref HEAD`.
func currentBranch(ctx context.Context, path string) (string, error) {
	output, err := git.RunGitCommand(ctx, path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	branch := strings.TrimSpace(string(output))
	if branch == "" || branch == "HEAD" {
		return "", fmt.Errorf("cannot push from a detached HEAD state")
	}
	return branch, nil
}

// hasUpstreamConfigured checks whether the current branch has a
// remote-tracking branch configured via `git rev-parse --symbolic-full-name
// @{u}` (D-09). A non-zero exit (typically 128, "no upstream configured")
// is treated as "no upstream", not an execution error.
func hasUpstreamConfigured(ctx context.Context, path string) (bool, error) {
	_, err := git.RunGitCommand(ctx, path, "rev-parse", "--symbolic-full-name", "@{u}")
	if err != nil {
		return false, nil
	}
	return true, nil
}

// aheadBehindCount runs `git rev-list --count --left-right HEAD...@{u}` to
// determine the exact local ahead/behind divergence count against the
// configured upstream branch, entirely locally (D-09).
func aheadBehindCount(ctx context.Context, path string) (ahead, behind int, err error) {
	output, rerr := git.RunGitCommand(ctx, path, "rev-list", "--count", "--left-right", "HEAD...@{u}")
	if rerr != nil {
		return 0, 0, rerr
	}

	fields := strings.Fields(strings.TrimSpace(string(output)))
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("unexpected rev-list output: %q", string(output))
	}

	ahead, errA := strconv.Atoi(fields[0])
	behind, errB := strconv.Atoi(fields[1])
	if errA != nil || errB != nil {
		return 0, 0, fmt.Errorf("failed to parse rev-list ahead/behind counts: %q", string(output))
	}

	return ahead, behind, nil
}

func init() {
	pushCmd.Flags().IntVarP(&pushConcurrency, "concurrency", "j", 5, "Maximum number of concurrent push operations")
	pushCmd.Flags().DurationVar(&pushTimeout, "timeout", 2*time.Minute, "Timeout for each individual push operation")
	rootCmd.AddCommand(pushCmd)
}
