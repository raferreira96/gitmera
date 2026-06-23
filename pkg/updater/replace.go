package updater

import (
	"fmt"
	"os"
	"path/filepath"
)

// Replace atomically installs newBinary at targetPath, the location of the
// currently running gitmera executable, via a single rename, which is safe
// even while targetPath is the binary of the running process.
func Replace(targetPath string, newBinary []byte) error {
	dir := filepath.Dir(targetPath)

	tmp, err := os.CreateTemp(dir, "gitmera-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmp.Write(newBinary); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write new binary to %s: %w", tmpPath, err)
	}
	if err := tmp.Chmod(0755); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to make %s executable: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp file %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("failed to install new binary at %s: %w", targetPath, err)
	}
	return nil
}
