package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"gitmera/pkg/config"
	"gitmera/pkg/runner"
	"gitmera/pkg/ui"

	"github.com/spf13/cobra"
)

// commandSetup bundles the configuration, concurrency, timeout, context, and
// task list resolved by setupCommand, shared by every workspace-wide verb
// command (clone, pull, push, checkout, status).
type commandSetup struct {
	cfg         *config.Config
	concurrency int
	timeout     time.Duration
	ctx         context.Context
	cancel      context.CancelFunc
	tasks       []runner.RepoTask
}

// setupCommand performs the configuration loading, concurrency/timeout
// resolution, and task list construction shared by every workspace-wide verb
// command. The caller must call the returned commandSetup.cancel via defer.
func setupCommand(cmd *cobra.Command, concurrencyFlag int, timeoutFlag time.Duration, action string) (*commandSetup, error) {
	configPath, err := resolveConfigPath(cfgFile)
	if err != nil {
		return nil, err
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		return nil, err
	}

	concurrency, err := resolveConcurrency(cmd, concurrencyFlag, cfg)
	if err != nil {
		return nil, err
	}

	timeout, err := resolveTimeout(cmd, timeoutFlag, cfg)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), globalTimeoutFor(timeout))

	return &commandSetup{
		cfg:         cfg,
		concurrency: concurrency,
		timeout:     timeout,
		ctx:         ctx,
		cancel:      cancel,
		tasks:       buildSortedTasks(cfg, action),
	}, nil
}

// loadConfig opens and strictly parses/validates the workspace configuration
// file at configPath.
func loadConfig(configPath string) (*config.Config, error) {
	f, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open configuration file %q: %w", configPath, err)
	}
	defer f.Close()

	cfg, err := config.Load(f)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration file %q: %w", configPath, err)
	}
	return cfg, nil
}

// resolveConcurrency determines the worker pool size: the command's
// --concurrency flag if explicitly set, otherwise the config file's
// concurrency value, otherwise a default of 5. It rejects values below 1.
func resolveConcurrency(cmd *cobra.Command, flagValue int, cfg *config.Config) (int, error) {
	concurrency := 5
	if cmd.Flags().Changed("concurrency") {
		concurrency = flagValue
	} else if cfg.Concurrency != nil {
		concurrency = *cfg.Concurrency
	}

	if concurrency < 1 {
		return 0, fmt.Errorf("concurrency must be a positive integer greater than or equal to 1, got %d", concurrency)
	}
	return concurrency, nil
}

// resolveTimeout determines the per-repository operation timeout: the
// command's --timeout flag if explicitly set, otherwise the config file's
// timeout value, otherwise a default of 2 minutes. An unparseable config
// timeout is reported as an error rather than silently treated as zero.
func resolveTimeout(cmd *cobra.Command, flagValue time.Duration, cfg *config.Config) (time.Duration, error) {
	if cmd.Flags().Changed("timeout") {
		return flagValue, nil
	}
	if cfg.Timeout != nil {
		parsed, err := time.ParseDuration(*cfg.Timeout)
		if err != nil {
			return 0, fmt.Errorf("invalid timeout %q in configuration: %w", *cfg.Timeout, err)
		}
		return parsed, nil
	}
	return 2 * time.Minute, nil
}

// globalTimeoutFor computes the overall run deadline as 5x the
// per-repository timeout, capped at 10 minutes.
func globalTimeoutFor(timeout time.Duration) time.Duration {
	global := 5 * timeout
	if global > 10*time.Minute {
		return 10 * time.Minute
	}
	return global
}

// buildSortedTasks builds the list of repository tasks from the config's
// projects map, sorted by name for consistent, deterministic UI output.
func buildSortedTasks(cfg *config.Config, action string) []runner.RepoTask {
	names := make([]string, 0, len(cfg.Projects))
	for name := range cfg.Projects {
		names = append(names, name)
	}
	sort.Strings(names)

	tasks := make([]runner.RepoTask, 0, len(names))
	for _, name := range names {
		proj := cfg.Projects[name]
		tasks = append(tasks, runner.RepoTask{
			Name:   name,
			URI:    proj.Repo,
			Path:   proj.Path,
			Action: action,
		})
	}
	return tasks
}

// isCancellationErr reports whether err represents a task that was skipped
// due to context cancellation/deadline (its own or a sibling failure under
// fail-fast), as opposed to a genuine operation failure.
func isCancellationErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// reportResults prints a detailed error box for every genuine (non
// cancellation) failure and an inline warning line for every skipped
// repository that carries a reason in Stderr (e.g. Smart Push's Safe Abort,
// Safe Switch). It returns true if at least one repository failed for a
// reason other than cancellation.
func reportResults(results []runner.TaskResult, logger *ui.SafeLogger) bool {
	hasRealError := false
	for _, res := range results {
		switch {
		case res.Err != nil:
			if !isCancellationErr(res.Err) {
				hasRealError = true
				logger.LogErrorBox(res.RepoName, res.Err, res.Stderr)
			}
		case res.Skipped && res.Stderr != "":
			logger.Print(fmt.Sprintf("  ↳ %s: %s\n", res.RepoName, res.Stderr))
		}
	}
	return hasRealError
}
