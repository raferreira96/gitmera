package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"

	"gitmera/pkg/config"
	"gitmera/pkg/ui"

	"github.com/spf13/cobra"
)

var (
	cfgFile        string
	verbose        bool
	noColor        bool
	nonInteractive bool
	plain          bool
	version        = "dev"
)

var rootCmd = &cobra.Command{
	Use:           "gitmera",
	Short:         "Gitmera is a high-performance CLI orchestrator for multiple Git repositories.",
	Long:          `A concurrent Git subprocess wrapper that allows you to run Git commands across multiple repositories simultaneously.`,
	Version:       version,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := resolveConfigPath(cfgFile)
		if err != nil {
			return err
		}

		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open configuration file %q: %w", path, err)
		}
		defer f.Close()

		if _, err := config.Load(f); err != nil {
			return fmt.Errorf("invalid configuration file %q: %w", path, err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Found and validated configuration file: %s\n", path)
		return nil
	},
}

// resolveConfigPath determines which configuration file to use: the explicit
// --config/-c flag value if provided, otherwise it searches strictly in the
// current working directory for .gitmera.yaml then .gitmera.yml.
func resolveConfigPath(explicitPath string) (string, error) {
	if explicitPath != "" {
		if _, err := os.Stat(explicitPath); err != nil {
			return "", fmt.Errorf("configuration file %q not found: %w", explicitPath, err)
		}
		return explicitPath, nil
	}

	candidates := []string{".gitmera.yaml", ".gitmera.yml"}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no configuration file found (looked for %v in current directory)", candidates)
}

// isInteractiveMode determines whether a network/branch command (clone,
// pull, checkout, push) should drive the interactive Bubble Tea TUI (true)
// or fall back to plain sequential logging (false). The TUI is only used
// when out is an attached TTY AND neither --non-interactive nor --plain
// was passed (D-07).
func isInteractiveMode(out io.Writer) bool {
	if nonInteractive || plain {
		return false
	}
	return ui.IsInteractiveTerminal(out)
}

// Execute runs the root command, wrapping execution in a context that is
// cancelled on os.Interrupt (Ctrl+C/SIGINT). Cancellation cascades through
// cmd.Context() into the concurrent runner and its os/exec.CommandContext
// Git subprocesses, terminating them cleanly before the CLI exits (D-12).
func Execute() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	return rootCmd.ExecuteContext(ctx)
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path (default is .gitmera.yaml or .gitmera.yml in current directory)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored terminal output")
	rootCmd.PersistentFlags().BoolVar(&nonInteractive, "non-interactive", false, "disable the interactive Bubble Tea progress UI and use sequential text logs")
	rootCmd.PersistentFlags().BoolVar(&plain, "plain", false, "alias for --non-interactive: force plain sequential text output")
}
