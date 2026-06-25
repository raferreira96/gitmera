package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gitmera/pkg/git"
)

// mockCheckoutScenario describes the simulated git command outputs for a
// single repository directory during a mocked checkout run.
type mockCheckoutScenario struct {
	statusOutput string
	statusExit   int

	showRefExit int // 0 = branch exists, non-zero = branch does not exist

	checkoutOutput string
	checkoutExit   int
}

// withMockGitCheckout installs a subprocess mock keyed by repository
// directory basename, routing `git` invocations to a re-exec'd
// TestCheckoutHelperProcess. Mirrors the pattern established in
// status_test.go's withMockGitStatus.
func withMockGitCheckout(t *testing.T, scenarios map[string]mockCheckoutScenario) {
	t.Helper()

	restore := git.SetExecCommandForTest(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestCheckoutHelperProcess", "--"}
		cs = append(cs, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		env := append(os.Environ(), "GO_WANT_CHECKOUT_HELPER_PROCESS=1")
		for repoName, sc := range scenarios {
			prefix := "GITMERA_MOCK_" + sanitizeEnvKey(repoName) + "_"
			env = append(env,
				prefix+"STATUS_OUT="+base64.StdEncoding.EncodeToString([]byte(sc.statusOutput)),
				prefix+"STATUS_EXIT="+strconv.Itoa(sc.statusExit),
				prefix+"SHOWREF_EXIT="+strconv.Itoa(sc.showRefExit),
				prefix+"CHECKOUT_OUT="+base64.StdEncoding.EncodeToString([]byte(sc.checkoutOutput)),
				prefix+"CHECKOUT_EXIT="+strconv.Itoa(sc.checkoutExit),
			)
		}
		cmd.Env = env
		return cmd
	})
	t.Cleanup(restore)
}

// TestCheckoutHelperProcess is the subprocess entry point used by
// withMockGitCheckout. It inspects the current working directory (set via
// exec.Cmd.Dir by git.RunGitCommand) to select the right scenario, then
// emits the configured mock output for the requested git subcommand.
func TestCheckoutHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_CHECKOUT_HELPER_PROCESS") != "1" {
		return
	}

	args := os.Args
	gitArgs := []string{}
	for i, arg := range args {
		if arg == "--" {
			gitArgs = args[i+1:]
			break
		}
	}
	if len(gitArgs) == 0 {
		fmt.Fprintln(os.Stderr, "no git arguments provided")
		os.Exit(1)
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to get cwd:", err)
		os.Exit(1)
	}
	repoName := filepath.Base(cwd)
	prefix := "GITMERA_MOCK_" + sanitizeEnvKey(repoName) + "_"

	subCmd := gitArgs[0]
	switch subCmd {
	case "status":
		out, derr := base64.StdEncoding.DecodeString(os.Getenv(prefix + "STATUS_OUT"))
		if derr != nil {
			fmt.Fprintf(os.Stderr, "no scenario configured for repo %q\n", repoName)
			os.Exit(1)
		}
		exitCode, _ := strconv.Atoi(os.Getenv(prefix + "STATUS_EXIT"))
		fmt.Print(string(out))
		os.Exit(exitCode)
	case "show-ref":
		exitCode, _ := strconv.Atoi(os.Getenv(prefix + "SHOWREF_EXIT"))
		os.Exit(exitCode)
	case "checkout":
		out, derr := base64.StdEncoding.DecodeString(os.Getenv(prefix + "CHECKOUT_OUT"))
		if derr != nil {
			fmt.Fprintf(os.Stderr, "no scenario configured for repo %q\n", repoName)
			os.Exit(1)
		}
		exitCode, _ := strconv.Atoi(os.Getenv(prefix + "CHECKOUT_EXIT"))
		fmt.Print(string(out))
		os.Exit(exitCode)
	default:
		fmt.Fprintf(os.Stderr, "unexpected git subcommand %q\n", subCmd)
		os.Exit(1)
	}
}

func TestCheckoutRepo_SwitchesCleanRepo(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "clean-repo", true)

	withMockGitCheckout(t, map[string]mockCheckoutScenario{
		"clean-repo": {
			statusOutput: "",
		},
	})

	warning, skipped, err := checkoutRepo(context.Background(), dir, "main", false)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if warning != "" {
		t.Errorf("expected no warning, got %q", warning)
	}
	if skipped {
		t.Error("expected skipped=false")
	}
}

func TestCheckoutRepo_SafeFailOnTrackedChanges(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "dirty-repo", true)

	withMockGitCheckout(t, map[string]mockCheckoutScenario{
		"dirty-repo": {
			statusOutput: " M main.go\n",
		},
	})

	_, _, err := checkoutRepo(context.Background(), dir, "main", false)

	if err == nil {
		t.Fatal("expected an error due to uncommitted tracked changes")
	}
	if !strings.Contains(err.Error(), "local changes would be overwritten") {
		t.Errorf("expected Safe Fail error message, got %q", err.Error())
	}
}

func TestCheckoutRepo_UntrackedFilesDoNotBlockCheckout(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "untracked-repo", true)

	withMockGitCheckout(t, map[string]mockCheckoutScenario{
		"untracked-repo": {
			statusOutput: "?? new_file.go\n",
		},
	})

	_, _, err := checkoutRepo(context.Background(), dir, "main", false)

	if err != nil {
		t.Errorf("expected untracked-only changes to not block checkout, got error: %v", err)
	}
}

func TestCheckoutRepo_CreateNewBranch(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "new-branch-repo", true)

	withMockGitCheckout(t, map[string]mockCheckoutScenario{
		"new-branch-repo": {
			statusOutput: "",
			showRefExit:  1, // branch does not exist
		},
	})

	warning, _, err := checkoutRepo(context.Background(), dir, "feature/x", true)

	if err != nil {
		t.Errorf("expected no error creating new branch, got %v", err)
	}
	if warning != "" {
		t.Errorf("expected no warning when creating a genuinely new branch, got %q", warning)
	}
}

func TestCheckoutRepo_SafeSwitchWhenBranchAlreadyExists(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "existing-branch-repo", true)

	withMockGitCheckout(t, map[string]mockCheckoutScenario{
		"existing-branch-repo": {
			statusOutput: "",
			showRefExit:  0, // branch already exists
		},
	})

	warning, _, err := checkoutRepo(context.Background(), dir, "feature/x", true)

	if err != nil {
		t.Errorf("expected Safe Switch fallback to succeed without error, got %v", err)
	}
	if !strings.Contains(warning, "already exists") {
		t.Errorf("expected a warning mentioning the branch already exists, got %q", warning)
	}
}

func TestCheckoutRepo_MissingRepoFails(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "missing-repo")

	_, _, err := checkoutRepo(context.Background(), dir, "main", false)

	if err == nil {
		t.Fatal("expected an error for a missing/non-git repository")
	}
}

func TestCheckoutRepo_CheckoutCommandFailureSurfacesError(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "fail-checkout-repo", true)

	withMockGitCheckout(t, map[string]mockCheckoutScenario{
		"fail-checkout-repo": {
			statusOutput:   "",
			checkoutOutput: "error: pathspec 'missing-branch' did not match any file(s) known to git",
			checkoutExit:   1,
		},
	})

	_, _, err := checkoutRepo(context.Background(), dir, "missing-branch", false)

	if err == nil {
		t.Fatal("expected an error when the underlying git checkout command fails")
	}
}

func TestHasUncommittedTrackedChanges(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "mixed-repo", true)

	withMockGitCheckout(t, map[string]mockCheckoutScenario{
		"mixed-repo": {
			statusOutput: "?? untracked.go\n M tracked.go\n",
		},
	})

	dirty, err := hasUncommittedTrackedChanges(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !dirty {
		t.Error("expected dirty=true when tracked modifications are present alongside untracked files")
	}
}

func TestLocalBranchExists(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "branch-check-repo", true)

	withMockGitCheckout(t, map[string]mockCheckoutScenario{
		"branch-check-repo": {
			showRefExit: 0,
		},
	})

	exists, err := localBranchExists(context.Background(), dir, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected exists=true when show-ref exits 0")
	}
}

func TestCheckoutRepo_CreateBranchCheckoutFails(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "create-fail-repo", true)

	withMockGitCheckout(t, map[string]mockCheckoutScenario{
		"create-fail-repo": {
			statusOutput:   "",
			showRefExit:    1, // branch does not exist → git checkout -b
			checkoutOutput: "error: could not create branch",
			checkoutExit:   1,
		},
	})

	_, _, err := checkoutRepo(context.Background(), dir, "new-feature", true)
	if err == nil {
		t.Fatal("expected error when branch creation (-b checkout) fails")
	}
}

func TestCheckoutRepo_StatusCommandError(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "status-cmd-fail-repo", true)

	withMockGitCheckout(t, map[string]mockCheckoutScenario{
		"status-cmd-fail-repo": {
			statusOutput: "",
			statusExit:   1, // git status fails
		},
	})

	_, _, err := checkoutRepo(context.Background(), dir, "main", false)
	if err == nil {
		t.Fatal("expected error when hasUncommittedTrackedChanges returns error via checkoutRepo")
	}
}

func TestHasUncommittedTrackedChanges_EmptyLineIgnored(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "empty-line-repo", true)

	// Status output with an internal empty line — only untracked files present.
	withMockGitCheckout(t, map[string]mockCheckoutScenario{
		"empty-line-repo": {
			statusOutput: "?? file1.go\n\n?? file2.go\n",
		},
	})

	dirty, err := hasUncommittedTrackedChanges(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dirty {
		t.Error("expected dirty=false when only untracked files (with empty lines) are present")
	}
}

func TestCheckoutRepo_ValidateDestinationError(t *testing.T) {
	base := t.TempDir()
	// Create a file (not directory) at the repo path to trigger a ValidateDestination error.
	repoPath := filepath.Join(base, "file-not-dir")
	if err := os.WriteFile(repoPath, []byte("not a dir"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	_, _, err := checkoutRepo(context.Background(), repoPath, "main", false)
	if err == nil {
		t.Fatal("expected an error when destination is a file, not a git repo")
	}
}

func TestCheckoutRepo_SafeSwitch_CheckoutFailure(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "safe-switch-fail-repo", true)

	withMockGitCheckout(t, map[string]mockCheckoutScenario{
		"safe-switch-fail-repo": {
			statusOutput:   "",
			showRefExit:    0, // branch already exists
			checkoutOutput: "error: failed to switch",
			checkoutExit:   1,
		},
	})

	_, _, err := checkoutRepo(context.Background(), dir, "feature/x", true)
	if err == nil {
		t.Fatal("expected error when Safe Switch checkout command fails")
	}
}

func TestHasUncommittedTrackedChanges_GitError(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "git-error-repo", true)

	withMockGitCheckout(t, map[string]mockCheckoutScenario{
		"git-error-repo": {
			statusOutput: "",
			statusExit:   128, // git status fails
		},
	})

	_, err := hasUncommittedTrackedChanges(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error when git status fails")
	}
}

func TestLocalBranchExists_NotFound(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "branch-missing-repo", true)

	withMockGitCheckout(t, map[string]mockCheckoutScenario{
		"branch-missing-repo": {
			showRefExit: 1,
		},
	})

	exists, err := localBranchExists(context.Background(), dir, "nope")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected exists=false when show-ref exits non-zero")
	}
}
