package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitmera/pkg/config"
	"gitmera/pkg/ui"
)

func TestNewWizard_NotNil(t *testing.T) {
	w := NewWizard(strings.NewReader(""), &bytes.Buffer{})
	if w == nil {
		t.Fatal("expected non-nil wizard")
	}
}

func TestWizard_PromptString_ReturnsInput(t *testing.T) {
	in := strings.NewReader("my-project\n")
	var out bytes.Buffer
	w := NewWizard(in, &out)

	got, err := w.PromptString("Project name", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "my-project" {
		t.Errorf("expected %q, got %q", "my-project", got)
	}
}

func TestWizard_PromptString_EmptyReturnsDefault(t *testing.T) {
	in := strings.NewReader("\n")
	var out bytes.Buffer
	w := NewWizard(in, &out)

	got, err := w.PromptString("Project name", "default-value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "default-value" {
		t.Errorf("expected default %q, got %q", "default-value", got)
	}
}

func TestWizard_PromptString_EmptyNoDefault(t *testing.T) {
	in := strings.NewReader("\n")
	var out bytes.Buffer
	w := NewWizard(in, &out)

	got, err := w.PromptString("Repo URL", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	// no hint should appear when defaultValue is empty
	if strings.Contains(out.String(), "[") {
		t.Errorf("expected no default hint, got: %q", out.String())
	}
}

func TestWizard_PromptString_WithEOFInput(t *testing.T) {
	in := strings.NewReader("myvalue")
	var out bytes.Buffer
	w := NewWizard(in, &out)

	// No newline, just EOF
	got, err := w.PromptString("Field", "fallback")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "myvalue" {
		t.Errorf("expected %q, got %q", "myvalue", got)
	}
}

func TestWizard_PromptConfirm_YesInput(t *testing.T) {
	in := strings.NewReader("y\n")
	var out bytes.Buffer
	w := NewWizard(in, &out)

	got, err := w.PromptConfirm("Continue?", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected true for 'y' input")
	}
}

func TestWizard_PromptConfirm_YesFullInput(t *testing.T) {
	in := strings.NewReader("yes\n")
	var out bytes.Buffer
	w := NewWizard(in, &out)

	got, err := w.PromptConfirm("Continue?", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected true for 'yes' input")
	}
}

func TestWizard_PromptConfirm_NoInput(t *testing.T) {
	in := strings.NewReader("n\n")
	var out bytes.Buffer
	w := NewWizard(in, &out)

	got, err := w.PromptConfirm("Continue?", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected false for 'n' input")
	}
}

func TestWizard_PromptConfirm_DefaultYes(t *testing.T) {
	in := strings.NewReader("\n")
	var out bytes.Buffer
	w := NewWizard(in, &out)

	got, err := w.PromptConfirm("Continue?", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected true for empty input with defaultYes=true")
	}
	if !strings.Contains(out.String(), "[Y/n]") {
		t.Errorf("expected [Y/n] hint, got: %q", out.String())
	}
}

func TestWizard_PromptConfirm_DefaultNo(t *testing.T) {
	in := strings.NewReader("\n")
	var out bytes.Buffer
	w := NewWizard(in, &out)

	got, err := w.PromptConfirm("Continue?", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected false for empty input with defaultYes=false")
	}
	if !strings.Contains(out.String(), "[y/N]") {
		t.Errorf("expected [y/N] hint, got: %q", out.String())
	}
}

// errReader is an io.Reader that always returns the configured error.
type errReader struct{ err error }

func (e errReader) Read(p []byte) (int, error) { return 0, e.err }

func TestWizard_PromptString_ReaderError(t *testing.T) {
	r := errReader{err: io.ErrUnexpectedEOF} // any non-EOF error
	var out bytes.Buffer
	w := NewWizard(r, &out)

	_, err := w.PromptString("Field", "default")
	if err == nil {
		t.Fatal("expected error when reader returns non-EOF error")
	}
}

func TestWizard_PromptConfirm_ReaderError(t *testing.T) {
	r := errReader{err: io.ErrUnexpectedEOF}
	var out bytes.Buffer
	w := NewWizard(r, &out)

	_, err := w.PromptConfirm("Continue?", false)
	if err == nil {
		t.Fatal("expected error when reader returns non-EOF error")
	}
}

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
// restoring the previous values afterward.
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

// setInitTestGlobals sets up package globals for init command tests and registers
// cleanup via t.Cleanup.
func setInitTestGlobals(t *testing.T, cfgPath string, nonInt, pln, frc bool) {
	t.Helper()
	prevCfg := cfgFile
	prevNI := nonInteractive
	prevPlain := plain
	prevForce := force
	cfgFile = cfgPath
	nonInteractive = nonInt
	plain = pln
	force = frc
	t.Cleanup(func() {
		cfgFile = prevCfg
		nonInteractive = prevNI
		plain = prevPlain
		force = prevForce
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

func TestInitCmd_NonInteractive_CreatesDefaultTemplate(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, ".gitmera.yaml")
	setInitTestGlobals(t, targetPath, true, false, false)

	var out bytes.Buffer
	initCmd.SetOut(&out)
	initCmd.SetIn(strings.NewReader(""))

	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(targetPath); err != nil {
		t.Fatalf("expected config file to be created: %v", err)
	}
	content, _ := os.ReadFile(targetPath)
	if !strings.Contains(string(content), "version:") {
		t.Errorf("expected 'version:' in config, got: %q", string(content))
	}
}

func TestInitCmd_NonInteractive_AbortIfExistsWithoutForce(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, ".gitmera.yaml")
	if err := os.WriteFile(targetPath, []byte("existing"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	setInitTestGlobals(t, targetPath, true, false, false)

	var out bytes.Buffer
	initCmd.SetOut(&out)
	initCmd.SetIn(strings.NewReader(""))

	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "already exists") {
		t.Errorf("expected abort message, got: %q", out.String())
	}
	// file must remain unchanged
	content, _ := os.ReadFile(targetPath)
	if string(content) != "existing" {
		t.Error("expected file content to be unchanged after non-interactive abort")
	}
}

func TestInitCmd_Force_OverwritesExistingConfig(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, ".gitmera.yaml")
	if err := os.WriteFile(targetPath, []byte("old content"), 0644); err != nil {
		t.Fatalf("failed to create existing file: %v", err)
	}
	setInitTestGlobals(t, targetPath, true, false, true)

	var out bytes.Buffer
	initCmd.SetOut(&out)
	initCmd.SetIn(strings.NewReader(""))

	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content, _ := os.ReadFile(targetPath)
	if string(content) == "old content" {
		t.Error("expected file to be overwritten with --force")
	}
	if !strings.Contains(string(content), "version:") {
		t.Errorf("expected valid config, got: %q", string(content))
	}
}

func TestInitCmd_Interactive_WizardCreatesConfig(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, ".gitmera.yaml")
	setInitTestGlobals(t, targetPath, false, false, false)

	// EOF after path acts as "n" to "Add another repository?" (default=false).
	stdin := strings.NewReader("myapi\ngit@github.com:example/myapi.git\n./myapi\n")
	var out bytes.Buffer
	initCmd.SetIn(stdin)
	initCmd.SetOut(&out)

	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("expected config to be created: %v", err)
	}
	if !strings.Contains(string(content), "myapi") {
		t.Errorf("expected project name in config, got: %q", string(content))
	}
}

func TestInitCmd_Interactive_ConfirmOverwriteYes(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, ".gitmera.yaml")
	if err := os.WriteFile(targetPath, []byte("old"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	setInitTestGlobals(t, targetPath, false, false, false)

	// Answers: confirm=y, name=api, repo=..., path=./api; EOF → don't add another.
	stdin := strings.NewReader("y\napi\ngit@github.com:example/api.git\n./api\n")
	var out bytes.Buffer
	initCmd.SetIn(stdin)
	initCmd.SetOut(&out)

	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content, _ := os.ReadFile(targetPath)
	if string(content) == "old" {
		t.Error("expected file to be overwritten after 'y' confirmation")
	}
}

func TestInitCmd_Interactive_DeclineOverwriteAborts(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, ".gitmera.yaml")
	original := "original content"
	if err := os.WriteFile(targetPath, []byte(original), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	setInitTestGlobals(t, targetPath, false, false, false)

	// Answer 'n' to overwrite confirmation
	stdin := strings.NewReader("n\n")
	var out bytes.Buffer
	initCmd.SetIn(stdin)
	initCmd.SetOut(&out)

	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "aborted") {
		t.Errorf("expected abort message, got: %q", out.String())
	}
	content, _ := os.ReadFile(targetPath)
	if string(content) != original {
		t.Error("expected file to remain unchanged after declining overwrite")
	}
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
