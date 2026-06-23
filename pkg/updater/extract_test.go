package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"
)

func buildTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(content))}); err != nil {
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

func TestExtractBinary_TarGz(t *testing.T) {
	archive := buildTarGz(t, "gitmera", []byte("fake-binary-content"))

	got, err := ExtractBinary(archive, "gitmera")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "fake-binary-content" {
		t.Errorf("got %q, want %q", got, "fake-binary-content")
	}
}

func TestExtractBinary_NotFound(t *testing.T) {
	archive := buildTarGz(t, "some-other-file", []byte("irrelevant"))

	_, err := ExtractBinary(archive, "gitmera")
	if err == nil {
		t.Fatal("expected an error when the binary isn't present, got nil")
	}
}
