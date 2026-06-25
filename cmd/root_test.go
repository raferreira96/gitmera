package cmd

import (
	"bytes"
	"os"
	"path/filepath"
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

func TestExecute_DoesNotError(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--version"})
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetArgs(nil)
	})

	if err := Execute(); err != nil {
		t.Fatalf("unexpected error from Execute(): %v", err)
	}
	if !strings.Contains(buf.String(), version) {
		t.Errorf("expected version %q in Execute() output, got %q", version, buf.String())
	}
}

func TestResolveConfigPath_ExplicitPathFound(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "myconfig.yaml")
	if err := os.WriteFile(cfgPath, []byte("version: \"1\"\nprojects: {}\n"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	got, err := resolveConfigPath(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != cfgPath {
		t.Errorf("expected %q, got %q", cfgPath, got)
	}
}

func TestResolveConfigPath_ExplicitPathNotFound(t *testing.T) {
	_, err := resolveConfigPath("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected an error for a nonexistent explicit path, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestResolveConfigPath_DefaultYamlFallback(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".gitmera.yaml")
	if err := os.WriteFile(cfgPath, []byte("version: \"1\"\nprojects: {}\n"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Change working directory to the temp dir so the default lookup finds it.
	prevDir, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevDir) })

	got, err := resolveConfigPath("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ".gitmera.yaml" {
		t.Errorf("expected .gitmera.yaml, got %q", got)
	}
}

func TestResolveConfigPath_DefaultYmlFallback(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".gitmera.yml")
	if err := os.WriteFile(cfgPath, []byte("version: \"1\"\nprojects: {}\n"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	prevDir, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevDir) })

	got, err := resolveConfigPath("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ".gitmera.yml" {
		t.Errorf("expected .gitmera.yml, got %q", got)
	}
}

func TestResolveConfigPath_NoneFound(t *testing.T) {
	dir := t.TempDir()
	prevDir, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevDir) })

	_, err := resolveConfigPath("")
	if err == nil {
		t.Fatal("expected an error when no config file exists, got nil")
	}
	if !strings.Contains(err.Error(), "no configuration file found") {
		t.Errorf("expected 'no configuration file found' in error, got: %v", err)
	}
}

func TestIsInteractiveMode_NonInteractiveFlag(t *testing.T) {
	prev := nonInteractive
	nonInteractive = true
	t.Cleanup(func() { nonInteractive = prev })

	var buf bytes.Buffer
	if isInteractiveMode(&buf) {
		t.Error("expected isInteractiveMode=false when --non-interactive is set")
	}
}

func TestIsInteractiveMode_PlainFlag(t *testing.T) {
	prev := plain
	plain = true
	t.Cleanup(func() { plain = prev })

	var buf bytes.Buffer
	if isInteractiveMode(&buf) {
		t.Error("expected isInteractiveMode=false when --plain is set")
	}
}

func TestIsInteractiveMode_NonTTY(t *testing.T) {
	prev := nonInteractive
	prevPlain := plain
	nonInteractive = false
	plain = false
	t.Cleanup(func() {
		nonInteractive = prev
		plain = prevPlain
	})

	// bytes.Buffer is not an *os.File, so IsInteractiveTerminal returns false.
	var buf bytes.Buffer
	if isInteractiveMode(&buf) {
		t.Error("expected isInteractiveMode=false for a non-TTY writer")
	}
}
