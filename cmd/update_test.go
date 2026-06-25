package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gitmera/pkg/updater"
)

// withUpdateOverrides points the update command's HTTP client/base URL and
// target path at test doubles, restoring the originals (and the version
// var) on test cleanup.
func withUpdateOverrides(t *testing.T, baseURL, targetPath string) {
	t.Helper()

	prevBaseURL := updateBaseURL
	prevClient := updateHTTPClient
	prevTarget := updateTargetPath
	prevVersion := version

	updateBaseURL = baseURL
	updateHTTPClient = http.DefaultClient
	updateTargetPath = func() (string, error) { return targetPath, nil }

	t.Cleanup(func() {
		updateBaseURL = prevBaseURL
		updateHTTPClient = prevClient
		updateTargetPath = prevTarget
		version = prevVersion
	})
}

func buildTestArchive(t *testing.T, binaryName string, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	if err := tw.WriteHeader(&tar.Header{Name: binaryName, Mode: 0755, Size: int64(len(content))}); err != nil {
		t.Fatalf("failed to write tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("failed to write tar content: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	return buf.Bytes()
}

func TestUpdateCmd_AlreadyUpToDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			t.Errorf("unexpected request to %s; should not download anything when already up to date", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name": "v0.5.0", "assets": []}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	targetPath := filepath.Join(dir, "gitmera")
	withUpdateOverrides(t, server.URL, targetPath)
	version = "v0.5.0"

	var out bytes.Buffer
	updateCmd.SetOut(&out)

	if err := updateCmd.RunE(updateCmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := out.String(); got != "gitmera is already at the latest version (v0.5.0)\n" {
		t.Errorf("output = %q, want the already-up-to-date message", got)
	}
}

func TestUpdateCmd_PerformsUpdate(t *testing.T) {
	assetName := updater.AssetName("v0.6.0", runtime.GOOS, runtime.GOARCH)
	archive := buildTestArchive(t, "gitmera", []byte("new-binary-content"))

	sum := sha256.Sum256(archive)
	checksums := []byte(fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), assetName))

	mux := http.NewServeMux()
	var server *httptest.Server
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"tag_name": "v0.6.0", "assets": [
			{"name": %q, "browser_download_url": %q},
			{"name": "checksums.txt", "browser_download_url": %q}
		]}`, assetName, server.URL+"/assets/"+assetName, server.URL+"/assets/checksums.txt")
	})
	mux.HandleFunc("/assets/"+assetName, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("/assets/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(checksums)
	})
	server = httptest.NewServer(mux)
	defer server.Close()

	dir := t.TempDir()
	targetPath := filepath.Join(dir, "gitmera")
	if err := os.WriteFile(targetPath, []byte("old-binary-content"), 0755); err != nil {
		t.Fatalf("failed to seed initial binary: %v", err)
	}

	withUpdateOverrides(t, server.URL, targetPath)
	version = "v0.5.0"

	var out bytes.Buffer
	updateCmd.SetOut(&out)

	if err := updateCmd.RunE(updateCmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read updated binary: %v", err)
	}
	if string(got) != "new-binary-content" {
		t.Errorf("installed binary content = %q, want %q", got, "new-binary-content")
	}

	if outStr := out.String(); !strings.Contains(outStr, "Updated to v0.6.0") {
		t.Errorf("output = %q, want it to mention the update to v0.6.0", outStr)
	}
}

func TestDefaultUpdateTargetPath_ReturnsNonEmpty(t *testing.T) {
	path, err := defaultUpdateTargetPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty executable path")
	}
}

func TestUpdateCmd_AssetNotFoundInRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Release with no assets matching the current platform.
		_, _ = w.Write([]byte(`{"tag_name": "v9.9.9", "assets": []}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	targetPath := filepath.Join(dir, "gitmera")
	withUpdateOverrides(t, server.URL, targetPath)
	version = "v0.1.0" // current is older → would need to update

	var out bytes.Buffer
	updateCmd.SetOut(&out)

	err := updateCmd.RunE(updateCmd, nil)
	if err == nil {
		t.Fatal("expected error when release asset is not found, got nil")
	}
	if !strings.Contains(err.Error(), "failed to locate release asset") {
		t.Errorf("expected 'failed to locate release asset', got: %v", err)
	}
}

func TestUpdateCmd_ChecksumsMissing(t *testing.T) {
	// Build the asset name matching the current runtime to ensure the first
	// AssetDownloadURL call succeeds and the second (checksums.txt) fails.
	assetName := updater.AssetName("v9.9.9", runtime.GOOS, runtime.GOARCH)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Release has the platform archive asset but no checksums.txt.
		_, _ = fmt.Fprintf(w, `{"tag_name": "v9.9.9", "assets": [{"name": %q, "browser_download_url": "http://example.com/asset"}]}`, assetName)
	}))
	defer server.Close()

	dir := t.TempDir()
	targetPath := filepath.Join(dir, "gitmera")
	withUpdateOverrides(t, server.URL, targetPath)
	version = "v0.1.0"

	var out bytes.Buffer
	updateCmd.SetOut(&out)

	err := updateCmd.RunE(updateCmd, nil)
	if err == nil {
		t.Fatal("expected error when checksums.txt is missing from release, got nil")
	}
	if !strings.Contains(err.Error(), "failed to locate checksums") {
		t.Errorf("expected 'failed to locate checksums', got: %v", err)
	}
}

func TestUpdateCmd_ArchiveDownloadFails(t *testing.T) {
	assetName := updater.AssetName("v9.9.9", runtime.GOOS, runtime.GOARCH)

	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"tag_name": "v9.9.9", "assets": [
			{"name": %q, "browser_download_url": %q},
			{"name": "checksums.txt", "browser_download_url": %q}
		]}`, assetName, srv.URL+"/assets/"+assetName, srv.URL+"/assets/checksums.txt")
	})
	mux.HandleFunc("/assets/"+assetName, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	targetPath := filepath.Join(dir, "gitmera")
	withUpdateOverrides(t, srv.URL, targetPath)
	version = "v0.1.0"

	var out bytes.Buffer
	updateCmd.SetOut(&out)

	err := updateCmd.RunE(updateCmd, nil)
	if err == nil {
		t.Fatal("expected error when archive download fails, got nil")
	}
	if !strings.Contains(err.Error(), "failed to download") {
		t.Errorf("expected 'failed to download', got: %v", err)
	}
}

func TestUpdateCmd_ChecksumsDownloadFails(t *testing.T) {
	assetName := updater.AssetName("v9.9.9", runtime.GOOS, runtime.GOARCH)
	archive := buildTestArchive(t, "gitmera", []byte("binary-content"))

	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"tag_name": "v9.9.9", "assets": [
			{"name": %q, "browser_download_url": %q},
			{"name": "checksums.txt", "browser_download_url": %q}
		]}`, assetName, srv.URL+"/assets/"+assetName, srv.URL+"/assets/checksums.txt")
	})
	mux.HandleFunc("/assets/"+assetName, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("/assets/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	targetPath := filepath.Join(dir, "gitmera")
	withUpdateOverrides(t, srv.URL, targetPath)
	version = "v0.1.0"

	var out bytes.Buffer
	updateCmd.SetOut(&out)

	err := updateCmd.RunE(updateCmd, nil)
	if err == nil {
		t.Fatal("expected error when checksums download fails, got nil")
	}
	if !strings.Contains(err.Error(), "failed to download checksums") {
		t.Errorf("expected 'failed to download checksums', got: %v", err)
	}
}

func TestUpdateCmd_ChecksumVerificationFails(t *testing.T) {
	assetName := updater.AssetName("v9.9.9", runtime.GOOS, runtime.GOARCH)
	archive := buildTestArchive(t, "gitmera", []byte("binary-content"))

	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"tag_name": "v9.9.9", "assets": [
			{"name": %q, "browser_download_url": %q},
			{"name": "checksums.txt", "browser_download_url": %q}
		]}`, assetName, srv.URL+"/assets/"+assetName, srv.URL+"/assets/checksums.txt")
	})
	mux.HandleFunc("/assets/"+assetName, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("/assets/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		// Return a checksum that does not match the archive.
		_, _ = w.Write([]byte(strings.Repeat("0", 64) + "  " + assetName + "\n"))
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	targetPath := filepath.Join(dir, "gitmera")
	withUpdateOverrides(t, srv.URL, targetPath)
	version = "v0.1.0"

	var out bytes.Buffer
	updateCmd.SetOut(&out)

	err := updateCmd.RunE(updateCmd, nil)
	if err == nil {
		t.Fatal("expected error when checksum verification fails, got nil")
	}
	if !strings.Contains(err.Error(), "checksum verification failed") {
		t.Errorf("expected 'checksum verification failed', got: %v", err)
	}
}

func TestUpdateCmd_FetchErrorIsWrapped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	dir := t.TempDir()
	targetPath := filepath.Join(dir, "gitmera")
	withUpdateOverrides(t, server.URL, targetPath)
	version = "v0.5.0"

	var out bytes.Buffer
	updateCmd.SetOut(&out)

	err := updateCmd.RunE(updateCmd, nil)
	if err == nil {
		t.Fatal("expected an error when the release API returns a 500, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "failed to check for updates") {
		t.Errorf("error = %q, want it to mention \"failed to check for updates\"", got)
	}
}
