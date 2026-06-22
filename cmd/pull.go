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
	pullConcurrency int
	pullTimeout     time.Duration
	pullFailFast    bool
)

var pullCmd = &cobra.Command{
	Use:          "pull",
	Short:        "Pull updates in all configured repositories concurrently",
	Long:         `Reads the workspace configuration and runs git pull concurrently in all child repositories, using a bounded worker pool and bypassing interactive credential prompts.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := ui.NewSafeLogger(cmd.OutOrStdout(), noColor)

		setup, err := setupCommand(cmd, pullConcurrency, pullTimeout, "pull")
		if err != nil {
			return err
		}
		defer setup.cancel()

		action := func(workerCtx context.Context, task runner.RepoTask) (error, string, bool) {
			// Validate destination path (must be a valid git repo)
			valid, err := git.ValidateDestination(task.Path)
			if err != nil {
				return err, "", false
			}
			if !valid {
				return fmt.Errorf("destination path %q does not exist or is not a valid Git repository", task.Path), "", false
			}

			output, err := git.RunGitCommand(workerCtx, task.Path, "pull")
			if err != nil {
				return err, string(output), false
			}
			return nil, string(output), false
		}

		interactive := isInteractiveMode(cmd.OutOrStdout())
		results := ui.OrchestrateExecution(setup.ctx, setup.tasks, setup.concurrency, pullFailFast, setup.timeout, action, ui.ExecutionOptions{
			Interactive: interactive,
			ActionLabel: "pulling",
			Logger:      logger,
		})

		// The TUI/fallback already printed a status line per repository
		// (success, skipped, cancelled) as events were dispatched; only
		// detailed error boxes for failures are printed here, at the end
		// of the run, per D-13.
		if reportResults(results, logger) || setup.ctx.Err() != nil {
			return fmt.Errorf("pull failed: one or more repositories failed to pull")
		}

		return nil
	},
}

func init() {
	pullCmd.Flags().IntVarP(&pullConcurrency, "concurrency", "j", 5, "Maximum number of concurrent pull operations")
	pullCmd.Flags().DurationVar(&pullTimeout, "timeout", 2*time.Minute, "Timeout for each individual pull operation")
	pullCmd.Flags().BoolVar(&pullFailFast, "fail-fast", false, "Abort all remaining operations on the first pull failure")
	rootCmd.AddCommand(pullCmd)
}
