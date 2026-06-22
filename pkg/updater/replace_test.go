package updater

import (
	"os"
	"path/filepath"
	"runtime"
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

	if runtime.GOOS != "windows" {
		info, err := os.Stat(targetPath)
		if err != nil {
			t.Fatalf("failed to stat replaced binary: %v", err)
		}
		if info.Mode().Perm()&0100 == 0 {
			t.Errorf("expected replaced binary to be executable, got mode %v", info.Mode())
		}
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
