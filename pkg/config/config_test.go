package config_test

import (
	"strings"
	"testing"

	"gitmera/pkg/config"
)

func TestLoad_Valid(t *testing.T) {
	input := `
version: "1"
projects:
  api:
    repo: "git@github.com:example/api.git"
    path: "./api"
  web:
    repo: "https://github.com/example/web.git"
    path: "src/web"
`
	cfg, err := config.Load(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if cfg.Version != "1" {
		t.Errorf("Expected version 1, got %q", cfg.Version)
	}

	if len(cfg.Projects) != 2 {
		t.Errorf("Expected 2 projects, got %d", len(cfg.Projects))
	}
}

func TestLoad_StrictCheck(t *testing.T) {
	input := `
version: "1"
unknown_field: "value"
projects:
  api:
    repo: "git@github.com:example/api.git"
    path: "./api"
`
	_, err := config.Load(strings.NewReader(input))
	if err == nil {
		t.Error("Expected strict parsing error due to unknown_field, got nil")
	}
}

func TestLoad_PathTraversalProtection(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"Absolute path", "/etc/passwd"},
		{"Relative parent jump", "../outside"},
		{"Double parent jump", "sub/../../outside"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Config{
				Version: "1",
				Projects: map[string]config.ProjectConfig{
					"bad": {Repo: "git@github.com:example/bad.git", Path: tt.path},
				},
			}
			err := cfg.Validate()
			if err == nil {
				t.Errorf("Expected path traversal error for path %q, got nil", tt.path)
			}
		})
	}
}

func TestLoad_ConcurrencyAndTimeout(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		expectedC   int
		expectedT   string
	}{
		{
			name: "Valid custom concurrency and timeout",
			input: `
version: "1"
concurrency: 3
timeout: "5m"
projects:
  api:
    repo: "git@github.com:example/api.git"
    path: "./api"
`,
			expectError: false,
			expectedC:   3,
			expectedT:   "5m",
		},
		{
			name: "Zero concurrency",
			input: `
version: "1"
concurrency: 0
projects:
  api:
    repo: "git@github.com:example/api.git"
    path: "./api"
`,
			expectError: true,
		},
		{
			name: "Negative concurrency",
			input: `
version: "1"
concurrency: -5
projects:
  api:
    repo: "git@github.com:example/api.git"
    path: "./api"
`,
			expectError: true,
		},
		{
			name: "Invalid timeout format",
			input: `
version: "1"
timeout: "invalid"
projects:
  api:
    repo: "git@github.com:example/api.git"
    path: "./api"
`,
			expectError: true,
		},
		{
			name: "Non-positive timeout",
			input: `
version: "1"
timeout: "0s"
projects:
  api:
    repo: "git@github.com:example/api.git"
    path: "./api"
`,
			expectError: true,
		},
		{
			name: "Negative timeout",
			input: `
version: "1"
timeout: "-5m"
projects:
  api:
    repo: "git@github.com:example/api.git"
    path: "./api"
`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.Load(strings.NewReader(tt.input))
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				if cfg.Concurrency == nil || *cfg.Concurrency != tt.expectedC {
					t.Errorf("expected concurrency %d, got %v", tt.expectedC, cfg.Concurrency)
				}
				if cfg.Timeout == nil || *cfg.Timeout != tt.expectedT {
					t.Errorf("expected timeout %q, got %v", tt.expectedT, cfg.Timeout)
				}
			}
		})
	}
}
