package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Replace atomically installs newBinary at targetPath, the location of the
// currently running gitmera executable. On unix, this is a single rename,
// which is safe even while targetPath is the binary of the running
// process. On windows, the running executable is locked, so the current
// binary is renamed aside first and best-effort removed afterward (a
// leftover ".old" file from a locked binary is harmless and gets cleaned up
// on a later run).
func Replace(targetPath string, newBinary []byte) error {
	dir := filepath.Dir(targetPath)

	tmp, err := os.CreateTemp(dir, "gitmera-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(newBinary); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to write new binary to %s: %w", tmpPath, err)
	}
	if err := tmp.Chmod(0755); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to make %s executable: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp file %s: %w", tmpPath, err)
	}

	if runtime.GOOS == "windows" {
		oldPath := targetPath + ".old"
		_ = os.Remove(oldPath)
		if err := os.Rename(targetPath, oldPath); err != nil {
			return fmt.Errorf("failed to move current binary aside at %s: %w", targetPath, err)
		}
		if err := os.Rename(tmpPath, targetPath); err != nil {
			return fmt.Errorf("failed to install new binary at %s: %w", targetPath, err)
		}
		_ = os.Remove(oldPath)
		return nil
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("failed to install new binary at %s: %w", targetPath, err)
	}
	return nil
}
