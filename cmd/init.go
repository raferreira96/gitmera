package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"gitmera/pkg/config"
	"gitmera/pkg/ui"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// nonInteractive is declared as a global persistent flag in root.go and
// shared across all subcommands (D-07); init reuses it to skip its
// interactive wizard prompts.
var force bool

// defaultConfigPath is the filename created by `gitmera init` when no
// explicit --config/-c path is provided.
const defaultConfigPath = ".gitmera.yaml"

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new gitmera workspace configuration",
	Long:  `Creates a .gitmera.yaml configuration file in the current directory, either interactively via a setup wizard or non-interactively with a default template.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		logger := ui.NewSafeLogger(out, noColor)

		// A single Wizard (and its single underlying bufio.Reader) is
		// created once per invocation and reused across both the overwrite
		// confirmation prompt and the project-detail prompts below, so
		// piped/scripted stdin input is consumed in the correct order.
		wizard := NewWizard(cmd.InOrStdin(), out)

		// Determine target config file path: explicit --config/-c override,
		// otherwise the default .gitmera.yaml in the current directory.
		targetPath := defaultConfigPath
		if cfgFile != "" {
			targetPath = cfgFile
		}

		if _, err := os.Stat(targetPath); err == nil && !force {
			if nonInteractive {
				logger.Print(fmt.Sprintf("Configuration file %s already exists. Aborting (non-interactive mode, use --force to overwrite).\n", targetPath))
				return nil
			}

			confirm, err := wizard.PromptConfirm(fmt.Sprintf("Configuration file %s already exists. Overwrite?", targetPath), false)
			if err != nil {
				return fmt.Errorf("failed to read overwrite confirmation: %w", err)
			}
			if !confirm {
				logger.Print("Initialization aborted.\n")
				return nil
			}
		}

		var cfg config.Config
		cfg.Version = "1"
		cfg.Projects = make(map[string]config.ProjectConfig)

		if nonInteractive {
			cfg.Projects["example"] = config.ProjectConfig{
				Repo: "git@github.com:example/repo.git",
				Path: "./example",
			}
		} else {
			logger.Print("=== Gitmera Configuration Wizard ===\n\n")

			name, err := wizard.PromptString("Initial project name", "api")
			if err != nil {
				return fmt.Errorf("failed to read project name: %w", err)
			}
			repo, err := wizard.PromptString("Git Repository URL", "")
			if err != nil {
				return fmt.Errorf("failed to read repository URL: %w", err)
			}
			path, err := wizard.PromptString("Local subdirectory path", "./"+name)
			if err != nil {
				return fmt.Errorf("failed to read local subdirectory path: %w", err)
			}

			cfg.Projects[name] = config.ProjectConfig{
				Repo: repo,
				Path: path,
			}
		}

		// Strictly validate the generated configuration before writing.
		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("generated configuration is invalid: %w", err)
		}

		data, err := yaml.Marshal(&cfg)
		if err != nil {
			return fmt.Errorf("failed to format YAML: %w", err)
		}

		if err := os.WriteFile(targetPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write config file: %w", err)
		}

		logger.LogSuccess("gitmera", fmt.Sprintf("Successfully initialized configuration at %s", targetPath))
		return nil
	},
}

func init() {
	initCmd.Flags().BoolVarP(&force, "force", "f", false, "force configuration overwrite without confirmation")
	rootCmd.AddCommand(initCmd)
}

// Wizard parses interactive prompt requests from an io.Reader and writes
// prompts/output to an io.Writer, decoupling the init wizard's I/O from
// os.Stdin/os.Stdout so it can be tested with in-memory buffers.
//
// A single bufio.Reader is held for the lifetime of the Wizard (rather than
// constructed fresh per-prompt) because bufio.Reader eagerly buffers ahead
// from the underlying io.Reader: recreating it on every call would discard
// already-buffered-but-unread input whenever a piped/scripted source (e.g.
// multiple newline-separated answers piped via stdin) delivers more than
// one answer in a single read chunk, corrupting subsequent prompts.
type Wizard struct {
	in  *bufio.Reader
	out io.Writer
}

// NewWizard constructs a Wizard reading prompt responses from in and
// writing prompt text to out.
func NewWizard(in io.Reader, out io.Writer) *Wizard {
	return &Wizard{in: bufio.NewReader(in), out: out}
}

// PromptString prompts promptText (showing defaultValue as a hint, if any)
// and returns the trimmed user response, or defaultValue if the response is
// empty.
func (w *Wizard) PromptString(promptText string, defaultValue string) (string, error) {
	fmt.Fprint(w.out, promptText)
	if defaultValue != "" {
		fmt.Fprintf(w.out, " [%s]", defaultValue)
	}
	fmt.Fprint(w.out, ": ")

	input, err := w.in.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue, nil
	}
	return input, nil
}

// PromptConfirm prompts a yes/no confirmation (defaulting to defaultYes when
// the user enters an empty response) and returns the boolean result.
func (w *Wizard) PromptConfirm(promptText string, defaultYes bool) (bool, error) {
	suffix := " [y/N]"
	if defaultYes {
		suffix = " [Y/n]"
	}
	fmt.Fprintf(w.out, "%s%s: ", promptText, suffix)

	input, err := w.in.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return defaultYes, nil
	}
	return input == "y" || input == "yes", nil
}
