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
			concurrency = cloneConcurrency
		} else if cfg.Concurrency != nil {
			concurrency = *cfg.Concurrency
		}

		if concurrency < 1 {
			return fmt.Errorf("concurrency must be a positive integer greater than or equal to 1, got %d", concurrency)
		}

		// Resolve individual timeout
		timeout := 2 * time.Minute
		if cmd.Flags().Changed("timeout") {
			timeout = cloneTimeout
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
				Action: "clone",
			})
		}

		action := func(workerCtx context.Context, task runner.RepoTask) (error, string, bool) {
			// Validate destination path
			skip, err := git.ValidateDestination(task.Path)
			if err != nil {
				return err, "", false
			}
			if skip {
				return nil, "", true
			}

			output, err := git.RunGitCommand(workerCtx, "", "clone", task.URI, task.Path)
			if err != nil {
				return err, string(output), false
			}
			return nil, string(output), false
		}

		interactive := isInteractiveMode(cmd.OutOrStdout())
		results := ui.OrchestrateExecution(ctx, tasks, concurrency, cloneFailFast, timeout, action, ui.ExecutionOptions{
			Interactive: interactive,
			ActionLabel: "cloning",
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
