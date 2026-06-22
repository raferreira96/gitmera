package cmd

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gitmera/pkg/config"

	"github.com/spf13/cobra"
)

func TestResolveTimeout_InvalidConfigValueReturnsError(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Duration("timeout", 2*time.Minute, "")

	badTimeout := "not-a-duration"
	cfg := &config.Config{Timeout: &badTimeout}

	_, err := resolveTimeout(cmd, 2*time.Minute, cfg)
	if err == nil {
		t.Fatal("expected an error for an unparseable config timeout, got nil")
	}
}

func TestResolveTimeout_FlagTakesPrecedenceOverConfig(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Duration("timeout", 2*time.Minute, "")
	if err := cmd.Flags().Set("timeout", "30s"); err != nil {
		t.Fatalf("failed to set flag: %v", err)
	}

	cfgTimeout := "5m"
	cfg := &config.Config{Timeout: &cfgTimeout}

	got, err := resolveTimeout(cmd, 30*time.Second, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 30*time.Second {
		t.Errorf("expected flag value 30s to win, got %v", got)
	}
}

func TestResolveTimeout_FallsBackToDefaultWhenUnset(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Duration("timeout", 2*time.Minute, "")

	cfg := &config.Config{}

	got, err := resolveTimeout(cmd, 2*time.Minute, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 2*time.Minute {
		t.Errorf("expected default 2m, got %v", got)
	}
}

func TestIsCancellationErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"context.Canceled direct", context.Canceled, true},
		{"context.DeadlineExceeded direct", context.DeadlineExceeded, true},
		{"wrapped context.Canceled", fmt.Errorf("git command failed: %w", context.Canceled), true},
		{"wrapped context.DeadlineExceeded", fmt.Errorf("git command failed: %s: %w", "signal: killed", context.DeadlineExceeded), true},
		{"unrelated error", errors.New("boom"), false},
		{"nil error", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCancellationErr(tt.err); got != tt.want {
				t.Errorf("isCancellationErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
