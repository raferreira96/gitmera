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
