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

// force, when set via --force/-f, skips the overwrite confirmation prompt
// (or, in --non-interactive mode, the abort) when the target config file
// already exists.
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

			defaultName := "api"
			for {
				name, repo, path, err := promptProject(wizard, logger, cfg.Projects, defaultName)
				if err != nil {
					return err
				}

				cfg.Projects[name] = config.ProjectConfig{
					Repo: repo,
					Path: path,
				}
				logger.LogSuccess(name, "Project added to configuration")
				defaultName = ""

				addAnother, err := wizard.PromptConfirm("Add another repository?", false)
				if err != nil {
					return fmt.Errorf("failed to read add-another confirmation: %w", err)
				}
				if !addAnother {
					break
				}
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
	_, _ = fmt.Fprint(w.out, promptText)
	if defaultValue != "" {
		_, _ = fmt.Fprintf(w.out, " [%s]", defaultValue)
	}
	_, _ = fmt.Fprint(w.out, ": ")

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
	_, _ = fmt.Fprintf(w.out, "%s%s: ", promptText, suffix)

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

// promptProject prompts for a single project's name, Git repository URL,
// and local subdirectory path. The name prompt retries on an empty value
// or a name already present in existing, so two projects configured in
// the same wizard run can never collide or silently overwrite each other.
// defaultName seeds the name prompt's suggested default — pass "api" for
// the wizard's first project and "" for every subsequent one, since
// suggesting "api" again would just immediately collide with the first
// entry's name.
func promptProject(wizard *Wizard, logger *ui.SafeLogger, existing map[string]config.ProjectConfig, defaultName string) (name, repo, path string, err error) {
	promptText := "Initial project name"
	if defaultName == "" {
		promptText = "Project name"
	}

	for {
		name, err = wizard.PromptString(promptText, defaultName)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to read project name: %w", err)
		}
		name = strings.TrimSpace(name)

		if name == "" {
			logger.Print("Project name cannot be empty.\n")
			continue
		}
		if _, exists := existing[name]; exists {
			logger.Print(fmt.Sprintf("Project name %q is already used. Choose a different name.\n", name))
			continue
		}
		break
	}

	repo, err = wizard.PromptString("Git Repository URL", "")
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read repository URL: %w", err)
	}
	path, err = wizard.PromptString("Local subdirectory path", "./"+name)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read local subdirectory path: %w", err)
	}

	return name, repo, path, nil
}
