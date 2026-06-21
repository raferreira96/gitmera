package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"gitmera/pkg/config"
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

		configPath, err := resolveConfigPath(cfgFile)
		if err != nil {
			return err
		}

		f, err := os.Open(configPath)
		if err != nil {
			return fmt.Errorf("failed to open configuration file %q: %w", configPath, err)
		}
		defer f.Close()

		cfg, err := config.Load(f)
		if err != nil {
			return fmt.Errorf("invalid configuration file %q: %w", configPath, err)
		}

		// Resolve concurrency
		concurrency := 5
		if cmd.Flags().Changed("concurrency") {
			concurrency = pullConcurrency
		} else if cfg.Concurrency != nil {
			concurrency = *cfg.Concurrency
		}

		if concurrency < 1 {
			return fmt.Errorf("concurrency must be a positive integer greater than or equal to 1, got %d", concurrency)
		}

		// Resolve individual timeout
		timeout := 2 * time.Minute
		if cmd.Flags().Changed("timeout") {
			timeout = pullTimeout
		} else if cfg.Timeout != nil {
			parsed, _ := time.ParseDuration(*cfg.Timeout)
			timeout = parsed
		}

		// Proportional timeout with 10-minute ceiling by default
		globalTimeout := 5 * timeout
		if globalTimeout > 10*time.Minute {
			globalTimeout = 10 * time.Minute
		}

		// Set up global context
		ctx, cancel := context.WithTimeout(cmd.Context(), globalTimeout)
		defer cancel()

		// Prepare tasks sorted by name for consistent UI log output
		var projectNames []string
		for name := range cfg.Projects {
			projectNames = append(projectNames, name)
		}
		sort.Strings(projectNames)

		var tasks []runner.RepoTask
		for _, name := range projectNames {
			proj := cfg.Projects[name]
			tasks = append(tasks, runner.RepoTask{
				Name:   name,
				URI:    proj.Repo,
				Path:   proj.Path,
				Action: "pull",
			})
		}

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
		results := ui.OrchestrateExecution(ctx, tasks, concurrency, pullFailFast, timeout, action, ui.ExecutionOptions{
			Interactive: interactive,
			ActionLabel: "pulling",
			Logger:      logger,
		})

		// The TUI/fallback already printed a status line per repository
		// (success, skipped, cancelled) as events were dispatched; only
		// detailed error boxes for failures are printed here, at the end
		// of the run, per D-13.
		hasRealError := false
		for _, res := range results {
			if res.Err != nil {
				isCancellation := (res.Err == context.Canceled || res.Err == context.DeadlineExceeded ||
					strings.Contains(res.Err.Error(), "context canceled") ||
					strings.Contains(res.Err.Error(), "context deadline exceeded"))

				if !isCancellation {
					hasRealError = true
					logger.LogErrorBox(res.RepoName, res.Err, res.Stderr)
				}
			}
		}

		if hasRealError || ctx.Err() != nil {
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
