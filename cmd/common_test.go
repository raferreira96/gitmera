package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitmera/pkg/config"
	"gitmera/pkg/runner"
	"gitmera/pkg/ui"

	"github.com/spf13/cobra"
)

// newTestSetupCmd returns a cobra.Command wired up with the flags that
// setupCommand expects, along with a Background context.
func newTestSetupCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Int("concurrency", 5, "")
	cmd.Flags().Duration("timeout", 2*time.Minute, "")
	cmd.SetContext(context.Background())
	return cmd
}

func TestSetupCommand_ConfigNotFound(t *testing.T) {
	prevCfgFile := cfgFile
	cfgFile = "/nonexistent/setup-config.yaml"
	t.Cleanup(func() { cfgFile = prevCfgFile })

	_, err := setupCommand(newTestSetupCmd(), 5, 2*time.Minute, "clone")
	if err == nil {
		t.Fatal("expected error when config file not found, got nil")
	}
}

func TestSetupCommand_InvalidConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "invalid.yaml")
	if err := os.WriteFile(cfgPath, []byte("invalid: yaml: [broken"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	prevCfgFile := cfgFile
	cfgFile = cfgPath
	t.Cleanup(func() { cfgFile = prevCfgFile })

	_, err := setupCommand(newTestSetupCmd(), 5, 2*time.Minute, "clone")
	if err == nil {
		t.Fatal("expected error for invalid YAML config, got nil")
	}
}

func TestSetupCommand_InvalidConcurrency(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bad-concurrency.yaml")
	content := "version: \"1\"\nconcurrency: -1\nprojects:\n  api:\n    repo: \"git@github.com:example/api.git\"\n    path: \"./api\"\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	prevCfgFile := cfgFile
	cfgFile = cfgPath
	t.Cleanup(func() { cfgFile = prevCfgFile })

	_, err := setupCommand(newTestSetupCmd(), 5, 2*time.Minute, "clone")
	if err == nil {
		t.Fatal("expected error for invalid concurrency, got nil")
	}
}

func TestSetupCommand_InvalidTimeout(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bad-timeout.yaml")
	content := "version: \"1\"\ntimeout: \"not-a-duration\"\nprojects:\n  api:\n    repo: \"git@github.com:example/api.git\"\n    path: \"./api\"\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	prevCfgFile := cfgFile
	cfgFile = cfgPath
	t.Cleanup(func() { cfgFile = prevCfgFile })

	_, err := setupCommand(newTestSetupCmd(), 5, 2*time.Minute, "clone")
	if err == nil {
		t.Fatal("expected error for invalid timeout, got nil")
	}
}

func TestSetupCommand_FlagConcurrencyNegative(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "valid.yaml")
	content := "version: \"1\"\nprojects:\n  api:\n    repo: \"git@github.com:example/api.git\"\n    path: \"./api\"\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	prevCfgFile := cfgFile
	cfgFile = cfgPath
	t.Cleanup(func() { cfgFile = prevCfgFile })

	cmd := newTestSetupCmd()
	// Set the concurrency flag to -1 so resolveConcurrency rejects it.
	if err := cmd.Flags().Set("concurrency", "-1"); err != nil {
		t.Fatalf("failed to set concurrency flag: %v", err)
	}

	_, err := setupCommand(cmd, -1, 2*time.Minute, "clone")
	if err == nil {
		t.Fatal("expected error for negative concurrency flag, got nil")
	}
	if !contains(err.Error(), "concurrency") {
		t.Errorf("expected concurrency error, got: %v", err)
	}
}

func TestSetupCommand_Success(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "valid.yaml")
	content := "version: \"1\"\nprojects:\n  api:\n    repo: \"git@github.com:example/api.git\"\n    path: \"./api\"\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	prevCfgFile := cfgFile
	cfgFile = cfgPath
	t.Cleanup(func() { cfgFile = prevCfgFile })

	setup, err := setupCommand(newTestSetupCmd(), 5, 2*time.Minute, "clone")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if setup == nil {
		t.Fatal("expected non-nil setup")
	}
	setup.cancel()
}

func TestResolveTimeout_InvalidConfigValueReturnsError(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Duration("timeout", 2*time.Minute, "")

	badTimeout := "not-a-duration"
	cfg := &config.Config{Timeout: &badTimeout}

	_, err := resolveTimeout(cmd, 2*time.Minute, cfg)
	if err == nil {
		t.Fatal("expected an error for an unparseable config timeout, got nil")
	}
}

func TestResolveTimeout_FlagTakesPrecedenceOverConfig(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Duration("timeout", 2*time.Minute, "")
	if err := cmd.Flags().Set("timeout", "30s"); err != nil {
		t.Fatalf("failed to set flag: %v", err)
	}

	cfgTimeout := "5m"
	cfg := &config.Config{Timeout: &cfgTimeout}

	got, err := resolveTimeout(cmd, 30*time.Second, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 30*time.Second {
		t.Errorf("expected flag value 30s to win, got %v", got)
	}
}

func TestResolveTimeout_FallsBackToDefaultWhenUnset(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Duration("timeout", 2*time.Minute, "")

	cfg := &config.Config{}

	got, err := resolveTimeout(cmd, 2*time.Minute, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 2*time.Minute {
		t.Errorf("expected default 2m, got %v", got)
	}
}

func TestResolveTimeout_ValidConfigTimeout(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Duration("timeout", 2*time.Minute, "")

	cfgTimeout := "5m"
	cfg := &config.Config{Timeout: &cfgTimeout}

	got, err := resolveTimeout(cmd, 2*time.Minute, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 5*time.Minute {
		t.Errorf("expected config timeout 5m, got %v", got)
	}
}

func TestResolveConcurrency_FromConfig(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Int("concurrency", 5, "")

	c := 3
	cfg := &config.Config{Concurrency: &c}

	got, err := resolveConcurrency(cmd, 5, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 3 {
		t.Errorf("expected concurrency from config (3), got %d", got)
	}
}

func TestResolveConcurrency_Default(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Int("concurrency", 5, "")

	cfg := &config.Config{}

	got, err := resolveConcurrency(cmd, 5, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 5 {
		t.Errorf("expected default concurrency (5), got %d", got)
	}
}

func TestResolveConcurrency_NegativeFromConfig(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Int("concurrency", 5, "")

	c := -1
	cfg := &config.Config{Concurrency: &c}

	_, err := resolveConcurrency(cmd, 5, cfg)
	if err == nil {
		t.Fatal("expected error for negative concurrency from config, got nil")
	}
}

func TestGlobalTimeoutFor_Short(t *testing.T) {
	// 5 * 1min = 5min < 10min → return 5min
	got := globalTimeoutFor(1 * time.Minute)
	if got != 5*time.Minute {
		t.Errorf("expected 5m, got %v", got)
	}
}

func TestGlobalTimeoutFor_Long(t *testing.T) {
	// 5 * 3min = 15min > 10min → cap at 10min
	got := globalTimeoutFor(3 * time.Minute)
	if got != 10*time.Minute {
		t.Errorf("expected 10m cap, got %v", got)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := loadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected an error for a nonexistent config file, got nil")
	}
	if !contains(err.Error(), "failed to open") {
		t.Errorf("expected 'failed to open' in error, got: %v", err)
	}
}

func TestLoadConfig_ValidFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".gitmera.yaml")
	content := `version: "1"
projects:
  api:
    repo: "git@github.com:example/api.git"
    path: "./api"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Version != "1" {
		t.Errorf("expected version 1, got %q", cfg.Version)
	}
}

func TestLoadConfig_InvalidContent(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".gitmera.yaml")
	if err := os.WriteFile(cfgPath, []byte("invalid: yaml: [broken"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := loadConfig(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestBuildSortedTasks_SortsAlphabetically(t *testing.T) {
	cfg := &config.Config{
		Version: "1",
		Projects: map[string]config.ProjectConfig{
			"zebra": {Repo: "git@github.com:org/zebra.git", Path: "./zebra"},
			"api":   {Repo: "git@github.com:org/api.git", Path: "./api"},
			"web":   {Repo: "git@github.com:org/web.git", Path: "./web"},
		},
	}

	tasks := buildSortedTasks(cfg, "clone")

	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	if tasks[0].Name != "api" || tasks[1].Name != "web" || tasks[2].Name != "zebra" {
		t.Errorf("expected alphabetical order [api, web, zebra], got [%s, %s, %s]",
			tasks[0].Name, tasks[1].Name, tasks[2].Name)
	}
	for _, task := range tasks {
		if task.Action != "clone" {
			t.Errorf("expected action 'clone', got %q for task %s", task.Action, task.Name)
		}
	}
}

func TestReportResults_NoErrors(t *testing.T) {
	var buf bytes.Buffer
	logger := ui.NewSafeLogger(&buf, true)

	results := []runner.TaskResult{
		{RepoName: "api", Err: nil, Skipped: false},
		{RepoName: "web", Err: nil, Skipped: true},
	}

	hasRealError := reportResults(results, logger)
	if hasRealError {
		t.Error("expected hasRealError=false when no genuine errors")
	}
}

func TestReportResults_RealError(t *testing.T) {
	var buf bytes.Buffer
	logger := ui.NewSafeLogger(&buf, true)

	results := []runner.TaskResult{
		{RepoName: "api", Err: errors.New("fatal: git failed"), Stderr: "stderr output"},
	}

	hasRealError := reportResults(results, logger)
	if !hasRealError {
		t.Error("expected hasRealError=true for a genuine error")
	}
	if !contains(buf.String(), "api") {
		t.Errorf("expected error box output to mention 'api', got: %q", buf.String())
	}
}

func TestReportResults_CancellationErrorNotCounted(t *testing.T) {
	var buf bytes.Buffer
	logger := ui.NewSafeLogger(&buf, true)

	results := []runner.TaskResult{
		{RepoName: "api", Err: fmt.Errorf("wrapped: %w", context.Canceled), Skipped: true},
	}

	hasRealError := reportResults(results, logger)
	if hasRealError {
		t.Error("expected hasRealError=false for a cancellation error")
	}
}

func TestReportResults_SkippedWithStderrPrintsWarning(t *testing.T) {
	var buf bytes.Buffer
	logger := ui.NewSafeLogger(&buf, true)

	results := []runner.TaskResult{
		{RepoName: "web", Err: nil, Skipped: true, Stderr: "branch already up-to-date"},
	}

	hasRealError := reportResults(results, logger)
	if hasRealError {
		t.Error("expected hasRealError=false for a skipped result")
	}
	if !contains(buf.String(), "web") || !contains(buf.String(), "branch already up-to-date") {
		t.Errorf("expected skip detail line, got: %q", buf.String())
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

func TestIsCancellationErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"context.Canceled direct", context.Canceled, true},
		{"context.DeadlineExceeded direct", context.DeadlineExceeded, true},
		{"wrapped context.Canceled", fmt.Errorf("git command failed: %w", context.Canceled), true},
		{"wrapped context.DeadlineExceeded", fmt.Errorf("git command failed: %s: %w", "signal: killed", context.DeadlineExceeded), true},
		{"unrelated error", errors.New("boom"), false},
		{"nil error", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCancellationErr(tt.err); got != tt.want {
				t.Errorf("isCancellationErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
