package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("archive-bytes"))
	}))
	defer server.Close()

	got, err := Download(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "archive-bytes" {
		t.Errorf("got %q, want %q", got, "archive-bytes")
	}
}

func TestDownload_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := Download(context.Background(), server.Client(), server.URL)
	if err == nil {
		t.Fatal("expected an error for a 404 response, got nil")
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("archive-bytes")
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	otherHash := strings.Repeat("0", 64)
	checksums := []byte(fmt.Sprintf("%s  gitmera_0.2.0_linux_amd64.tar.gz\n%s  checksums-other.tar.gz\n", hash, otherHash))

	if err := VerifyChecksum(checksums, "gitmera_0.2.0_linux_amd64.tar.gz", data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyChecksum_Mismatch(t *testing.T) {
	checksums := []byte(strings.Repeat("0", 64) + "  gitmera_0.2.0_linux_amd64.tar.gz\n")

	err := VerifyChecksum(checksums, "gitmera_0.2.0_linux_amd64.tar.gz", []byte("archive-bytes"))
	if err == nil {
		t.Fatal("expected a checksum mismatch error, got nil")
	}
}

func TestVerifyChecksum_MissingEntry(t *testing.T) {
	checksums := []byte(strings.Repeat("0", 64) + "  some-other-file.tar.gz\n")

	err := VerifyChecksum(checksums, "gitmera_0.2.0_linux_amd64.tar.gz", []byte("archive-bytes"))
	if err == nil {
		t.Fatal("expected a missing-entry error, got nil")
	}
}
