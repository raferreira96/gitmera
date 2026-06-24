package cmd

import (
	"strings"
	"testing"

	"gitmera/pkg/config"
	"gitmera/pkg/ui"
)

func TestPromptProject_ReturnsNameRepoPath(t *testing.T) {
	in := strings.NewReader("api\ngit@github.com:example/api.git\n./api\n")
	var out strings.Builder
	wizard := NewWizard(in, &out)
	logger := ui.NewSafeLogger(&out, true)

	name, repo, path, err := promptProject(wizard, logger, map[string]config.ProjectConfig{}, "api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "api" {
		t.Errorf("expected name %q, got %q", "api", name)
	}
	if repo != "git@github.com:example/api.git" {
		t.Errorf("expected repo %q, got %q", "git@github.com:example/api.git", repo)
	}
	if path != "./api" {
		t.Errorf("expected path %q, got %q", "./api", path)
	}
}

func TestPromptProject_RetriesOnEmptyName(t *testing.T) {
	// First name line is blank (no default suggested -> retries),
	// second line provides a valid name.
	in := strings.NewReader("\nworker\ngit@github.com:example/worker.git\n./worker\n")
	var out strings.Builder
	wizard := NewWizard(in, &out)
	logger := ui.NewSafeLogger(&out, true)

	name, _, _, err := promptProject(wizard, logger, map[string]config.ProjectConfig{}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "worker" {
		t.Errorf("expected retry to land on name %q, got %q", "worker", name)
	}
	if !strings.Contains(out.String(), "cannot be empty") {
		t.Errorf("expected an empty-name retry message, got output: %q", out.String())
	}
}

func TestPromptProject_RetriesOnDuplicateName(t *testing.T) {
	// "api" is already taken; second attempt "worker" succeeds.
	in := strings.NewReader("api\nworker\ngit@github.com:example/worker.git\n./worker\n")
	var out strings.Builder
	wizard := NewWizard(in, &out)
	logger := ui.NewSafeLogger(&out, true)

	existing := map[string]config.ProjectConfig{
		"api": {Repo: "git@github.com:example/api.git", Path: "./api"},
	}

	name, _, _, err := promptProject(wizard, logger, existing, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "worker" {
		t.Errorf("expected retry to land on name %q, got %q", "worker", name)
	}
	if !strings.Contains(out.String(), `"api" is already used`) {
		t.Errorf("expected a duplicate-name retry message, got output: %q", out.String())
	}
}
