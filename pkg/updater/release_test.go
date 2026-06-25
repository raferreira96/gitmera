package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchLatestRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			t.Errorf("unexpected request path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tag_name": "v0.2.0",
			"assets": [
				{"name": "gitmera_0.2.0_linux_amd64.tar.gz", "browser_download_url": "https://example.com/gitmera_0.2.0_linux_amd64.tar.gz"},
				{"name": "checksums.txt", "browser_download_url": "https://example.com/checksums.txt"}
			]
		}`))
	}))
	defer server.Close()

	release, err := FetchLatestRelease(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if release.TagName != "v0.2.0" {
		t.Errorf("TagName = %q, want %q", release.TagName, "v0.2.0")
	}
	if len(release.Assets) != 2 {
		t.Fatalf("got %d assets, want 2", len(release.Assets))
	}
	if release.Assets[0].Name != "gitmera_0.2.0_linux_amd64.tar.gz" {
		t.Errorf("Assets[0].Name = %q, want %q", release.Assets[0].Name, "gitmera_0.2.0_linux_amd64.tar.gz")
	}
}

func TestFetchLatestRelease_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := FetchLatestRelease(context.Background(), server.Client(), server.URL)
	if err == nil {
		t.Fatal("expected an error for a 500 response, got nil")
	}
}

func TestFetchLatestRelease_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer server.Close()

	_, err := FetchLatestRelease(context.Background(), server.Client(), server.URL)
	if err == nil {
		t.Fatal("expected error for invalid JSON response, got nil")
	}
}

func TestFetchLatestRelease_InvalidURL(t *testing.T) {
	// An invalid base URL causes the request build to fail.
	_, err := FetchLatestRelease(context.Background(), &http.Client{}, "://invalid-url")
	if err == nil {
		t.Fatal("expected error for invalid base URL, got nil")
	}
}

func TestFetchLatestRelease_DialFails(t *testing.T) {
	// Create a server, record its URL, then close it so connections are refused.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()

	_, err := FetchLatestRelease(context.Background(), http.DefaultClient, url)
	if err == nil {
		t.Fatal("expected error when server is unreachable, got nil")
	}
}

func TestAssetDownloadURL(t *testing.T) {
	release := Release{
		TagName: "v0.2.0",
		Assets: []Asset{
			{Name: "gitmera_0.2.0_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/a.tar.gz"},
			{Name: "checksums.txt", BrowserDownloadURL: "https://example.com/checksums.txt"},
		},
	}

	got, err := AssetDownloadURL(release, "checksums.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://example.com/checksums.txt" {
		t.Errorf("got %q, want %q", got, "https://example.com/checksums.txt")
	}

	_, err = AssetDownloadURL(release, "does-not-exist.tar.gz")
	if err == nil {
		t.Fatal("expected an error for a missing asset, got nil")
	}
}
