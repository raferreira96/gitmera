package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"gitmera/pkg/ui"
	"gitmera/pkg/updater"

	"github.com/spf13/cobra"
)

// updateBaseURL, updateHTTPClient, and updateTargetPath are package-level so
// tests can substitute a fake GitHub API server, a test HTTP client, and a
// throwaway file in place of the real running executable.
var (
	updateBaseURL    = updater.DefaultBaseURL
	updateHTTPClient = &http.Client{Timeout: 30 * time.Second}
	updateTargetPath = defaultUpdateTargetPath
)

func defaultUpdateTargetPath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to determine current executable path: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve current executable path: %w", err)
	}
	return resolved, nil
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update gitmera to the latest release",
	Long:  `Checks GitHub for the latest gitmera release and, if newer than the running version, downloads it, verifies its checksum, and replaces the currently running binary in place.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		logger := ui.NewSafeLogger(out, noColor)

		baseCtx := cmd.Context()
		if baseCtx == nil {
			// cmd.Context() is only populated once the command has been run
			// through Execute/ExecuteContext; direct RunE invocations (as in
			// this package's tests) leave it nil, so fall back safely.
			baseCtx = context.Background()
		}
		ctx, cancel := context.WithTimeout(baseCtx, 2*time.Minute)
		defer cancel()

		release, err := updater.FetchLatestRelease(ctx, updateHTTPClient, updateBaseURL)
		if err != nil {
			return fmt.Errorf("failed to check for updates: %w", err)
		}

		if updater.CompareVersions(version, release.TagName) >= 0 {
			logger.Print(fmt.Sprintf("gitmera is already at the latest version (%s)\n", release.TagName))
			return nil
		}

		assetName := updater.AssetName(release.TagName, runtime.GOOS, runtime.GOARCH)
		assetURL, err := updater.AssetDownloadURL(release, assetName)
		if err != nil {
			return fmt.Errorf("failed to locate release asset: %w", err)
		}
		checksumsURL, err := updater.AssetDownloadURL(release, "checksums.txt")
		if err != nil {
			return fmt.Errorf("failed to locate checksums file: %w", err)
		}

		logger.Print(fmt.Sprintf("Downloading %s...\n", assetName))
		archive, err := updater.Download(ctx, updateHTTPClient, assetURL)
		if err != nil {
			return fmt.Errorf("failed to download %s: %w", assetName, err)
		}

		checksums, err := updater.Download(ctx, updateHTTPClient, checksumsURL)
		if err != nil {
			return fmt.Errorf("failed to download checksums.txt: %w", err)
		}

		if err := updater.VerifyChecksum(checksums, assetName, archive); err != nil {
			return fmt.Errorf("checksum verification failed: %w", err)
		}

		binaryName := "gitmera"
		if runtime.GOOS == "windows" {
			binaryName = "gitmera.exe"
		}
		binary, err := updater.ExtractBinary(archive, assetName, binaryName)
		if err != nil {
			return fmt.Errorf("failed to extract %s from %s: %w", binaryName, assetName, err)
		}

		targetPath, err := updateTargetPath()
		if err != nil {
			return err
		}

		if err := updater.Replace(targetPath, binary); err != nil {
			return fmt.Errorf("failed to install update: %w", err)
		}

		logger.LogSuccess("gitmera", fmt.Sprintf("Updated to %s", release.TagName))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
