package updater

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    int
	}{
		{"equal with v prefix on latest only", "0.1.0", "v0.1.0", 0},
		{"equal both with v prefix", "v0.1.0", "v0.1.0", 0},
		{"current behind on patch", "0.1.0", "v0.1.1", -1},
		{"current behind on minor", "0.1.0", "v0.2.0", -1},
		{"current behind on major", "0.1.0", "v1.0.0", -1},
		{"current ahead", "0.2.0", "v0.1.0", 1},
		{"dev build is always outdated", "dev", "v0.1.0", -1},
		{"empty current is always outdated", "", "v0.1.0", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompareVersions(tt.current, tt.latest)
			if got != tt.want {
				t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

func TestCompareVersions_InvalidLatest(t *testing.T) {
	// When latest is malformed, parseVersion returns nil → CompareVersions returns 0.
	got := CompareVersions("1.0.0", "not-a-version")
	if got != 0 {
		t.Errorf("expected 0 for malformed latest, got %d", got)
	}
}

func TestCompareVersions_InvalidCurrent(t *testing.T) {
	// When current is neither "dev" nor a valid version, parseVersion returns nil → returns -1.
	got := CompareVersions("bad-version", "v1.0.0")
	if got != -1 {
		t.Errorf("expected -1 for malformed current version, got %d", got)
	}
}

func TestParseVersion_TwoParts(t *testing.T) {
	// A two-part version like "1.2" is invalid (requires exactly 3 parts).
	result := parseVersion("1.2")
	if result != nil {
		t.Errorf("expected nil for two-part version, got %v", result)
	}
}

func TestParseVersion_NonNumericPart(t *testing.T) {
	result := parseVersion("1.2.alpha")
	if result != nil {
		t.Errorf("expected nil for non-numeric patch, got %v", result)
	}
}

func TestAssetName(t *testing.T) {
	tests := []struct {
		name    string
		version string
		goos    string
		goarch  string
		want    string
	}{
		{"linux amd64 strips v prefix", "v0.2.0", "linux", "amd64", "gitmera_0.2.0_linux_amd64.tar.gz"},
		{"darwin arm64 without v prefix", "0.2.0", "darwin", "arm64", "gitmera_0.2.0_darwin_arm64.tar.gz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AssetName(tt.version, tt.goos, tt.goarch)
			if got != tt.want {
				t.Errorf("AssetName(%q, %q, %q) = %q, want %q", tt.version, tt.goos, tt.goarch, got, tt.want)
			}
		})
	}
}
