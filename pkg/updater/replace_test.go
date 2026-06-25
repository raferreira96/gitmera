package updater

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplace(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "gitmera")

	if err := os.WriteFile(targetPath, []byte("old-content"), 0644); err != nil {
		t.Fatalf("failed to seed initial binary: %v", err)
	}

	if err := Replace(targetPath, []byte("new-content")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read replaced binary: %v", err)
	}
	if string(got) != "new-content" {
		t.Errorf("content = %q, want %q", got, "new-content")
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("failed to stat replaced binary: %v", err)
	}
	if info.Mode().Perm()&0100 == 0 {
		t.Errorf("expected replaced binary to be executable, got mode %v", info.Mode())
	}
}

func TestReplace_CannotCreateTempFile(t *testing.T) {
	// Passing a target path whose parent directory does not exist triggers
	// os.CreateTemp to fail.
	err := Replace("/nonexistent/dir/gitmera", []byte("content"))
	if err == nil {
		t.Fatal("expected error when parent directory does not exist, got nil")
	}
}

func TestReplace_RenameFails(t *testing.T) {
	dir := t.TempDir()
	// targetPath is an existing directory — os.Rename from a file to a
	// directory fails with EISDIR on Linux/macOS, exercising the Rename error branch.
	targetPath := filepath.Join(dir, "existing-dir")
	if err := os.Mkdir(targetPath, 0755); err != nil {
		t.Fatalf("failed to create target dir: %v", err)
	}

	err := Replace(targetPath, []byte("content"))
	if err == nil {
		t.Fatal("expected error when renaming to an existing directory, got nil")
	}
}

func TestReplace_NoExistingBinary(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "gitmera")

	if err := Replace(targetPath, []byte("new-content")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read replaced binary: %v", err)
	}
	if string(got) != "new-content" {
		t.Errorf("content = %q, want %q", got, "new-content")
	}
}
