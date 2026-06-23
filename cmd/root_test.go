package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmd_VersionFlagPrintsVersion(t *testing.T) {
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{"--version"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out.String(), version) {
		t.Errorf("expected --version output to contain %q, got %q", version, out.String())
	}
}

// TestRootCmd_SilencesErrorsAndUsage guards against Cobra's default behavior
// of auto-printing "Error: ..." plus the full usage/help text whenever RunE
// returns an error. main.go already prints the error once itself, so
// rootCmd must keep SilenceErrors/SilenceUsage set or the failure message
// and help text get duplicated on the terminal. Per Cobra's ExecuteC, these
// two fields on the root command gate the behavior for every subcommand
// too, so asserting them here covers the whole CLI.
func TestRootCmd_SilencesErrorsAndUsage(t *testing.T) {
	if !rootCmd.SilenceErrors {
		t.Error("rootCmd.SilenceErrors must be true, otherwise Cobra duplicates the error message main.go already prints")
	}
	if !rootCmd.SilenceUsage {
		t.Error("rootCmd.SilenceUsage must be true, otherwise Cobra dumps the full usage/help text on every command failure")
	}
}
