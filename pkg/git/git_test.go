package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestValidateDestination(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gitmera-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Case 1: Path does not exist -> skip=false, err=nil
	nonExistent := filepath.Join(tmpDir, "non-existent")
	skip, err := ValidateDestination(nonExistent)
	if err != nil {
		t.Errorf("expected no error for non-existent path, got %v", err)
	}
	if skip {
		t.Error("expected skip=false for non-existent path")
	}

	// Case 2: Path exists as a file -> skip=false, err != nil
	filePath := filepath.Join(tmpDir, "some-file")
	err = os.WriteFile(filePath, []byte("hello"), 0644)
	if err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	skip, err = ValidateDestination(filePath)
	if err == nil {
		t.Error("expected error for path existing as a file, got nil")
	} else if !strings.Contains(err.Error(), "exists and is a file") {
		t.Errorf("unexpected error message: %v", err)
	}
	if skip {
		t.Error("expected skip=false when path is a file")
	}

	// Case 3: Path exists as invalid directory (no .git) -> skip=false, err != nil
	invalidDir := filepath.Join(tmpDir, "invalid-dir")
	err = os.Mkdir(invalidDir, 0755)
	if err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	skip, err = ValidateDestination(invalidDir)
	if err == nil {
		t.Error("expected error for directory missing .git, got nil")
	} else if !strings.Contains(err.Error(), "missing .git directory") {
		t.Errorf("unexpected error message: %v", err)
	}
	if skip {
		t.Error("expected skip=false when directory has no .git")
	}

	// Case 4: Path exists with valid .git directory -> skip=true, err=nil
	validRepoDir := filepath.Join(tmpDir, "valid-repo")
	err = os.Mkdir(validRepoDir, 0755)
	if err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	err = os.Mkdir(filepath.Join(validRepoDir, ".git"), 0755)
	if err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}
	skip, err = ValidateDestination(validRepoDir)
	if err != nil {
		t.Errorf("expected no error for valid repo path, got %v", err)
	}
	if !skip {
		t.Error("expected skip=true for valid repo path")
	}
}

func TestRunGitCommand(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		// Route command to the current test binary itself to trigger TestHelperProcess
		cmd := oldExec(ctx, os.Args[0], append([]string{"-test.run=TestHelperProcess", "--"}, args...)...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"MOCK_COMMAND_NAME="+name,
		)
		return cmd
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Success case and environment variable verification
	output, err := RunGitCommand(ctx, "", "status")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(string(output), "mocked git status") {
		t.Errorf("expected output to contain 'mocked git status', got %q", string(output))
	}

	// 2. Token sanitization case
	output, err = RunGitCommand(ctx, "", "clone", "https://secret-token-123@github.com/org/repo.git", "dest")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(string(output), "cloning https://[REDACTED]@github.com/org/repo.git") {
		t.Errorf("expected token to be sanitized, got %q", string(output))
	}

	// 3. Error case
	_, err = RunGitCommand(ctx, "", "fail-cmd")
	if err == nil {
		t.Error("expected error, got nil")
	} else if !strings.Contains(err.Error(), "git command failed") {
		t.Errorf("expected error to contain 'git command failed', got %v", err)
	}
}

func TestRunGitCommand_PreservesContextDeadlineExceeded(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmd := oldExec(ctx, os.Args[0], append([]string{"-test.run=TestHelperProcess", "--"}, args...)...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"MOCK_COMMAND_NAME="+name,
		)
		return cmd
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := RunGitCommand(ctx, "", "sleep")
	if err == nil {
		t.Fatal("expected error due to context deadline, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected errors.Is(err, context.DeadlineExceeded) to be true, got err=%v", err)
	}
}

func TestExecCommandOverride_ConcurrentAccess(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				restore := SetExecCommandForTest(oldExec)
				restore()
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = RunGitCommand(ctx, "", "status")
		}()
	}

	time.Sleep(150 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	cmdName := os.Getenv("MOCK_COMMAND_NAME")
	if cmdName != "git" {
		fmt.Fprintf(os.Stderr, "Expected command 'git', got %q\n", cmdName)
		os.Exit(1)
	}

	// Verify target environment variables
	if os.Getenv("GIT_TERMINAL_PROMPT") != "0" {
		fmt.Fprintf(os.Stderr, "Expected GIT_TERMINAL_PROMPT=0, got %q\n", os.Getenv("GIT_TERMINAL_PROMPT"))
		os.Exit(1)
	}
	sshCmd := os.Getenv("GIT_SSH_COMMAND")
	if !strings.Contains(sshCmd, "BatchMode=yes") || !strings.Contains(sshCmd, "StrictHostKeyChecking=accept-new") {
		fmt.Fprintf(os.Stderr, "Expected BatchMode/StrictHostKeyChecking, got GIT_SSH_COMMAND=%q\n", sshCmd)
		os.Exit(1)
	}

	// Extract args passed to "git"
	args := os.Args
	// args is: [binary, -test.run=TestHelperProcess, --, git, <actual git args...>]
	gitArgs := []string{}
	for i, arg := range args {
		if arg == "--" {
			gitArgs = args[i+1:]
			break
		}
	}

	if len(gitArgs) == 0 {
		fmt.Fprintf(os.Stderr, "No git arguments provided\n")
		os.Exit(1)
	}

	subCmd := gitArgs[0]
	switch subCmd {
	case "status":
		fmt.Println("mocked git status")
		os.Exit(0)
	case "clone":
		// Print back the clone command target (which should have the token to test sanitization)
		fmt.Printf("cloning %s\n", gitArgs[1])
		os.Exit(0)
	case "fail-cmd":
		fmt.Fprintf(os.Stderr, "fatal: error with token https://secret-token-123@github.com/org/repo.git\n")
		os.Exit(128)
	case "sleep":
		time.Sleep(2 * time.Second)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand %q\n", subCmd)
		os.Exit(1)
	}
}
