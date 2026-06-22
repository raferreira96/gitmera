package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmd_VersionFlagPrintsVersion(t *testing.T) {
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{"--version"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out.String(), version) {
		t.Errorf("expected --version output to contain %q, got %q", version, out.String())
	}
}
