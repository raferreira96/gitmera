package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitmera/pkg/config"
	"gitmera/pkg/ui"
)

func TestPromptProject_ReturnsNameRepoPath(t *testing.T) {
	in := strings.NewReader("api\ngit@github.com:example/api.git\n./api\n")
	var out strings.Builder
	wizard := NewWizard(in, &out)
	logger := ui.NewSafeLogger(&out, true)

	name, repo, path, err := promptProject(wizard, logger, map[string]config.ProjectConfig{}, "api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "api" {
		t.Errorf("expected name %q, got %q", "api", name)
	}
	if repo != "git@github.com:example/api.git" {
		t.Errorf("expected repo %q, got %q", "git@github.com:example/api.git", repo)
	}
	if path != "./api" {
		t.Errorf("expected path %q, got %q", "./api", path)
	}
}

func TestPromptProject_RetriesOnEmptyName(t *testing.T) {
	// First name line is blank (no default suggested -> retries),
	// second line provides a valid name.
	in := strings.NewReader("\nworker\ngit@github.com:example/worker.git\n./worker\n")
	var out strings.Builder
	wizard := NewWizard(in, &out)
	logger := ui.NewSafeLogger(&out, true)

	name, _, _, err := promptProject(wizard, logger, map[string]config.ProjectConfig{}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "worker" {
		t.Errorf("expected retry to land on name %q, got %q", "worker", name)
	}
	if !strings.Contains(out.String(), "cannot be empty") {
		t.Errorf("expected an empty-name retry message, got output: %q", out.String())
	}
}

func TestPromptProject_RetriesOnDuplicateName(t *testing.T) {
	// "api" is already taken; second attempt "worker" succeeds.
	in := strings.NewReader("api\nworker\ngit@github.com:example/worker.git\n./worker\n")
	var out strings.Builder
	wizard := NewWizard(in, &out)
	logger := ui.NewSafeLogger(&out, true)

	existing := map[string]config.ProjectConfig{
		"api": {Repo: "git@github.com:example/api.git", Path: "./api"},
	}

	name, _, _, err := promptProject(wizard, logger, existing, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "worker" {
		t.Errorf("expected retry to land on name %q, got %q", "worker", name)
	}
	if !strings.Contains(out.String(), `"api" is already used`) {
		t.Errorf("expected a duplicate-name retry message, got output: %q", out.String())
	}
}

// withInitTestState points the package-level flag vars consumed by
// initCmd.RunE at a fresh, isolated state for the duration of the test,
// restoring the previous values afterward. Mirrors the pattern in
// concurrency_guard_test.go's withConcurrencyGuardConfig.
func withInitTestState(t *testing.T, targetPath string) {
	t.Helper()
	prevCfgFile, prevNonInteractive, prevForce, prevNoColor := cfgFile, nonInteractive, force, noColor
	cfgFile = targetPath
	nonInteractive = false
	force = false
	noColor = true
	t.Cleanup(func() {
		cfgFile, nonInteractive, force, noColor = prevCfgFile, prevNonInteractive, prevForce, prevNoColor
	})
}

// loadWrittenConfig reads and parses the .gitmera.yaml file written by
// initCmd at path, failing the test on any read or parse error.
func loadWrittenConfig(t *testing.T, path string) *config.Config {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open written config: %v", err)
	}
	defer func() { _ = f.Close() }()

	cfg, err := config.Load(f)
	if err != nil {
		t.Fatalf("failed to parse written config: %v", err)
	}
	return cfg
}

func TestInitCmd_SingleRepository(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), ".gitmera.yaml")
	withInitTestState(t, targetPath)

	input := strings.NewReader("api\ngit@github.com:example/api.git\n./api\nn\n")
	var out strings.Builder
	initCmd.SetIn(input)
	initCmd.SetOut(&out)
	initCmd.SetContext(context.Background())

	if err := initCmd.RunE(initCmd, []string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg := loadWrittenConfig(t, targetPath)
	if len(cfg.Projects) != 1 {
		t.Fatalf("expected exactly 1 project, got %d: %+v", len(cfg.Projects), cfg.Projects)
	}
	if _, ok := cfg.Projects["api"]; !ok {
		t.Errorf("expected project %q in config, got %+v", "api", cfg.Projects)
	}
}

func TestInitCmd_MultipleRepositories(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), ".gitmera.yaml")
	withInitTestState(t, targetPath)

	input := strings.NewReader(
		"api\ngit@github.com:example/api.git\n./api\ny\n" +
			"worker\ngit@github.com:example/worker.git\n./worker\nn\n",
	)
	var out strings.Builder
	initCmd.SetIn(input)
	initCmd.SetOut(&out)
	initCmd.SetContext(context.Background())

	if err := initCmd.RunE(initCmd, []string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg := loadWrittenConfig(t, targetPath)
	if len(cfg.Projects) != 2 {
		t.Fatalf("expected exactly 2 projects, got %d: %+v", len(cfg.Projects), cfg.Projects)
	}
	for _, name := range []string{"api", "worker"} {
		if _, ok := cfg.Projects[name]; !ok {
			t.Errorf("expected project %q in config, got %+v", name, cfg.Projects)
		}
	}
}

func TestInitCmd_EnterDefaultsToStopAdding(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), ".gitmera.yaml")
	withInitTestState(t, targetPath)

	// Blank line for the "Add another repository?" prompt = Enter = default = stop.
	input := strings.NewReader("api\ngit@github.com:example/api.git\n./api\n\n")
	var out strings.Builder
	initCmd.SetIn(input)
	initCmd.SetOut(&out)
	initCmd.SetContext(context.Background())

	if err := initCmd.RunE(initCmd, []string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg := loadWrittenConfig(t, targetPath)
	if len(cfg.Projects) != 1 {
		t.Fatalf("expected the wizard to stop after 1 project on Enter, got %d: %+v", len(cfg.Projects), cfg.Projects)
	}
}
