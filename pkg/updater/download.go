package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Download performs an HTTP GET against url and returns the full response
// body.
func Download(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request for %s: %w", url, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d downloading %s", resp.StatusCode, url)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body from %s: %w", url, err)
	}
	return data, nil
}

// VerifyChecksum checks that the SHA-256 of data matches the entry for
// filename within checksumsTxt, a GoReleaser checksums.txt file
// ("<hex sha256>  <filename>" per line, whitespace-separated).
func VerifyChecksum(checksumsTxt []byte, filename string, data []byte) error {
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])

	for _, line := range strings.Split(string(checksumsTxt), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[1] != filename {
			continue
		}
		if fields[0] != got {
			return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", filename, fields[0], got)
		}
		return nil
	}

	return fmt.Errorf("no checksum entry found for %s", filename)
}
