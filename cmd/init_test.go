package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	// Answers: confirm=y, name=api, repo=..., path=./api
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
