package cmd

import (
	"context"
	"fmt"
	"time"

	"gitmera/pkg/git"
	"gitmera/pkg/runner"
	"gitmera/pkg/ui"

	"github.com/spf13/cobra"
)

var (
	cloneConcurrency int
	cloneTimeout     time.Duration
	cloneFailFast    bool
)

var cloneCmd = &cobra.Command{
	Use:          "clone",
	Short:        "Clone all configured repositories concurrently",
	Long:         `Reads the workspace configuration and clones all child repositories in parallel, using a bounded worker pool and bypassing interactive credential prompts.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := ui.NewSafeLogger(cmd.OutOrStdout(), noColor)

		setup, err := setupCommand(cmd, cloneConcurrency, cloneTimeout, "clone")
		if err != nil {
			return err
		}
		defer setup.cancel()

		action := func(workerCtx context.Context, task runner.RepoTask) (string, bool, error) {
			// Validate destination path
			alreadyCloned, err := git.ValidateDestination(task.Path)
			if err != nil {
				return "", false, err
			}
			if alreadyCloned {
				return "", true, nil
			}

			output, err := git.RunGitCommand(workerCtx, "", "clone", task.URI, task.Path)
			if err != nil {
				return string(output), false, err
			}
			return string(output), false, nil
		}

		interactive := isInteractiveMode(cmd.OutOrStdout())
		results := ui.OrchestrateExecution(setup.ctx, setup.tasks, setup.concurrency, cloneFailFast, setup.timeout, action, ui.ExecutionOptions{
			Interactive: interactive,
			ActionLabel: "cloning",
			Logger:      logger,
		})

		// The TUI/fallback already printed a status line per repository
		// (success, skipped, cancelled) as events were dispatched; only
		// detailed error boxes for failures are printed here, at the end
		// of the run, per D-13.
		if reportResults(results, logger) || setup.ctx.Err() != nil {
			return fmt.Errorf("clone failed: one or more repositories failed to clone")
		}

		return nil
	},
}

func init() {
	cloneCmd.Flags().IntVarP(&cloneConcurrency, "concurrency", "j", 5, "Maximum number of concurrent clone operations")
	cloneCmd.Flags().DurationVar(&cloneTimeout, "timeout", 2*time.Minute, "Timeout for each individual clone operation")
	cloneCmd.Flags().BoolVar(&cloneFailFast, "fail-fast", false, "Abort all remaining operations on the first clone failure")
	rootCmd.AddCommand(cloneCmd)
}
