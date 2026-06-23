package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// DefaultBaseURL is the GitHub API base URL for gitmera's own repository,
// used to look up the latest release.
const DefaultBaseURL = "https://api.github.com/repos/raferreira96/gitmera"

// Asset is a single downloadable file attached to a GitHub release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Release is the subset of the GitHub releases API response gitmera needs.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// FetchLatestRelease queries baseURL + "/releases/latest" and decodes the
// response into a Release.
func FetchLatestRelease(ctx context.Context, client *http.Client, baseURL string) (Release, error) {
	url := baseURL + "/releases/latest"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, fmt.Errorf("failed to build request for %s: %w", url, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("failed to fetch latest release from %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("unexpected status %d fetching latest release from %s", resp.StatusCode, url)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return Release{}, fmt.Errorf("failed to decode release response from %s: %w", url, err)
	}

	return release, nil
}

// AssetDownloadURL returns the BrowserDownloadURL of the asset named name
// within release, or an error if no such asset is present.
func AssetDownloadURL(release Release, name string) (string, error) {
	for _, asset := range release.Assets {
		if asset.Name == name {
			return asset.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("asset %q not found in release %s", name, release.TagName)
}
