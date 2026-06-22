// Package updater implements gitmera's self-update mechanism: fetching the
// latest GitHub release, verifying its checksum, and atomically replacing
// the currently running binary.
package updater

import (
	"fmt"
	"strconv"
	"strings"
)

// CompareVersions compares current against latest (each optionally prefixed
// with "v") and returns -1, 0, or 1, mirroring strings.Compare semantics. A
// current value of "dev" or "" (an unset/local build) is always treated as
// outdated so locally-built binaries can still update.
func CompareVersions(current, latest string) int {
	if current == "dev" || current == "" {
		return -1
	}

	c := parseVersion(current)
	if c == nil {
		return -1
	}

	l := parseVersion(latest)
	if l == nil {
		return 0
	}

	for i := 0; i < 3; i++ {
		if c[i] != l[i] {
			if c[i] < l[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// parseVersion parses a "v1.2.3" or "1.2.3" string into [major, minor,
// patch]. It returns nil if v isn't in that exact three-part numeric shape.
func parseVersion(v string) []int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return nil
	}

	out := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		out[i] = n
	}
	return out
}

// AssetName builds the GoReleaser archive filename for version (with or
// without a leading "v") on the given platform, matching the name_template
// in .goreleaser.yaml: gitmera_{version}_{goos}_{goarch}.tar.gz.
func AssetName(version, goos, goarch string) string {
	version = strings.TrimPrefix(version, "v")
	return fmt.Sprintf("gitmera_%s_%s_%s.tar.gz", version, goos, goarch)
}
