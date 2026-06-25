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

// mockPushScenario describes the simulated git command outputs for a single
// repository directory during a mocked push run.
type mockPushScenario struct {
	branchOutput string // output of `git rev-parse --abbrev-ref HEAD`
	branchExit   int

	upstreamExit int // exit code of `git rev-parse --symbolic-full-name @{u}` (0 = has upstream)

	revListOutput string // output of `git rev-list --count --left-right HEAD...@{u}`
	revListExit   int

	pushOutput string
	pushExit   int
}

// withMockGitPush installs a subprocess mock keyed by repository directory
// basename, routing `git` invocations to a re-exec'd TestPushHelperProcess.
// Mirrors the pattern established in status_test.go / checkout_test.go.
func withMockGitPush(t *testing.T, scenarios map[string]mockPushScenario) {
	t.Helper()

	restore := git.SetExecCommandForTest(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestPushHelperProcess", "--"}
		cs = append(cs, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		env := append(os.Environ(), "GO_WANT_PUSH_HELPER_PROCESS=1")
		for repoName, sc := range scenarios {
			prefix := "GITMERA_MOCK_" + sanitizeEnvKey(repoName) + "_"
			env = append(env,
				prefix+"BRANCH_OUT="+base64.StdEncoding.EncodeToString([]byte(sc.branchOutput)),
				prefix+"BRANCH_EXIT="+strconv.Itoa(sc.branchExit),
				prefix+"UPSTREAM_EXIT="+strconv.Itoa(sc.upstreamExit),
				prefix+"REVLIST_OUT="+base64.StdEncoding.EncodeToString([]byte(sc.revListOutput)),
				prefix+"REVLIST_EXIT="+strconv.Itoa(sc.revListExit),
				prefix+"PUSH_OUT="+base64.StdEncoding.EncodeToString([]byte(sc.pushOutput)),
				prefix+"PUSH_EXIT="+strconv.Itoa(sc.pushExit),
			)
		}
		cmd.Env = env
		return cmd
	})
	t.Cleanup(restore)
}

// TestPushHelperProcess is the subprocess entry point used by
// withMockGitPush. It inspects the current working directory (set via
// exec.Cmd.Dir by git.RunGitCommand) to select the right scenario, then
// emits the configured mock output for the requested git subcommand.
func TestPushHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_PUSH_HELPER_PROCESS") != "1" {
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
	case "rev-parse":
		// Disambiguate between `rev-parse --abbrev-ref HEAD` (current
		// branch) and `rev-parse --symbolic-full-name @{u}` (upstream
		// check) based on the second argument.
		if len(gitArgs) >= 2 && gitArgs[1] == "--abbrev-ref" {
			out, derr := base64.StdEncoding.DecodeString(os.Getenv(prefix + "BRANCH_OUT"))
			if derr != nil {
				fmt.Fprintf(os.Stderr, "no scenario configured for repo %q\n", repoName)
				os.Exit(1)
			}
			exitCode, _ := strconv.Atoi(os.Getenv(prefix + "BRANCH_EXIT"))
			fmt.Print(string(out))
			os.Exit(exitCode)
		}
		if len(gitArgs) >= 2 && gitArgs[1] == "--symbolic-full-name" {
			exitCode, _ := strconv.Atoi(os.Getenv(prefix + "UPSTREAM_EXIT"))
			if exitCode != 0 {
				fmt.Fprintln(os.Stderr, "fatal: no upstream configured")
			} else {
				fmt.Println("refs/remotes/origin/" + "main")
			}
			os.Exit(exitCode)
		}
		fmt.Fprintf(os.Stderr, "unexpected rev-parse args %v\n", gitArgs)
		os.Exit(1)
	case "rev-list":
		out, derr := base64.StdEncoding.DecodeString(os.Getenv(prefix + "REVLIST_OUT"))
		if derr != nil {
			fmt.Fprintf(os.Stderr, "no scenario configured for repo %q\n", repoName)
			os.Exit(1)
		}
		exitCode, _ := strconv.Atoi(os.Getenv(prefix + "REVLIST_EXIT"))
		fmt.Print(string(out))
		os.Exit(exitCode)
	case "push":
		out, derr := base64.StdEncoding.DecodeString(os.Getenv(prefix + "PUSH_OUT"))
		if derr != nil {
			fmt.Fprintf(os.Stderr, "no scenario configured for repo %q\n", repoName)
			os.Exit(1)
		}
		exitCode, _ := strconv.Atoi(os.Getenv(prefix + "PUSH_EXIT"))
		fmt.Print(string(out))
		os.Exit(exitCode)
	default:
		fmt.Fprintf(os.Stderr, "unexpected git subcommand %q\n", subCmd)
		os.Exit(1)
	}
}

func TestPushRepo_SkipsWhenUpToDate(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "uptodate-repo", true)

	withMockGitPush(t, map[string]mockPushScenario{
		"uptodate-repo": {
			branchOutput:  "main\n",
			upstreamExit:  0,
			revListOutput: "0\t0\n",
		},
	})

	reason, skipped, err := pushRepo(context.Background(), dir)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !skipped {
		t.Error("expected skipped=true for an up-to-date branch")
	}
	if reason == "" {
		t.Error("expected a non-empty skip reason")
	}
}

func TestPushRepo_PushesWhenAhead(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "ahead-repo", true)

	withMockGitPush(t, map[string]mockPushScenario{
		"ahead-repo": {
			branchOutput:  "main\n",
			upstreamExit:  0,
			revListOutput: "3\t0\n",
		},
	})

	_, skipped, err := pushRepo(context.Background(), dir)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if skipped {
		t.Error("expected skipped=false when ahead of remote")
	}
}

func TestPushRepo_SafeAbortWhenBehind(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "behind-repo", true)

	withMockGitPush(t, map[string]mockPushScenario{
		"behind-repo": {
			branchOutput:  "main\n",
			upstreamExit:  0,
			revListOutput: "0\t2\n",
		},
	})

	reason, skipped, err := pushRepo(context.Background(), dir)

	if err != nil {
		t.Errorf("expected no error (Safe Abort is not a failure), got %v", err)
	}
	if !skipped {
		t.Error("expected skipped=true when behind remote")
	}
	if reason == "" {
		t.Error("expected a detailed warning reason when behind remote")
	}
}

func TestPushRepo_SafeAbortWhenDiverged(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "diverged-repo", true)

	withMockGitPush(t, map[string]mockPushScenario{
		"diverged-repo": {
			branchOutput:  "main\n",
			upstreamExit:  0,
			revListOutput: "3\t4\n",
		},
	})

	reason, skipped, err := pushRepo(context.Background(), dir)

	if err != nil {
		t.Errorf("expected no error (Safe Abort is not a failure), got %v", err)
	}
	if !skipped {
		t.Error("expected skipped=true when diverged from remote")
	}
	if reason == "" {
		t.Error("expected a detailed warning reason when diverged")
	}
}

func TestPushRepo_AutoSetsUpstreamWhenMissing(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "no-upstream-repo", true)

	withMockGitPush(t, map[string]mockPushScenario{
		"no-upstream-repo": {
			branchOutput: "feature/new\n",
			upstreamExit: 128, // no upstream configured
		},
	})

	_, skipped, err := pushRepo(context.Background(), dir)

	if err != nil {
		t.Errorf("expected no error when auto-setting upstream, got %v", err)
	}
	if skipped {
		t.Error("expected skipped=false when auto-setting upstream and pushing")
	}
}

func TestPushRepo_MissingRepoFails(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "missing-repo")

	_, _, err := pushRepo(context.Background(), dir)

	if err == nil {
		t.Fatal("expected an error for a missing/non-git repository")
	}
}

func TestPushRepo_PushCommandFailureSurfacesError(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "fail-push-repo", true)

	withMockGitPush(t, map[string]mockPushScenario{
		"fail-push-repo": {
			branchOutput:  "main\n",
			upstreamExit:  0,
			revListOutput: "1\t0\n",
			pushOutput:    "remote: permission denied",
			pushExit:      1,
		},
	})

	_, _, err := pushRepo(context.Background(), dir)

	if err == nil {
		t.Fatal("expected an error when the underlying git push command fails")
	}
}

func TestCurrentBranch_DetachedHeadFails(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "detached-push-repo", true)

	withMockGitPush(t, map[string]mockPushScenario{
		"detached-push-repo": {
			branchOutput: "HEAD\n",
		},
	})

	_, err := currentBranch(context.Background(), dir)
	if err == nil {
		t.Fatal("expected an error when HEAD is detached")
	}
}

func TestAheadBehindCount_ParsesCorrectly(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "count-repo", true)

	withMockGitPush(t, map[string]mockPushScenario{
		"count-repo": {
			revListOutput: "5\t7\n",
		},
	})

	ahead, behind, err := aheadBehindCount(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ahead != 5 || behind != 7 {
		t.Errorf("expected ahead=5 behind=7, got ahead=%d behind=%d", ahead, behind)
	}
}

func TestHasUpstreamConfigured(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "upstream-check-repo", true)

	withMockGitPush(t, map[string]mockPushScenario{
		"upstream-check-repo": {
			upstreamExit: 0,
		},
	})

	has, err := hasUpstreamConfigured(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("expected hasUpstream=true when rev-parse exits 0")
	}
}

func TestCurrentBranch_EmptyOutputFails(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "empty-branch-repo", true)

	withMockGitPush(t, map[string]mockPushScenario{
		"empty-branch-repo": {
			branchOutput: "\n", // empty after TrimSpace
		},
	})

	_, err := currentBranch(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error for empty branch output")
	}
}

func TestAheadBehindCount_RevListError(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "revlist-error-repo", true)

	withMockGitPush(t, map[string]mockPushScenario{
		"revlist-error-repo": {
			revListOutput: "",
			revListExit:   128,
		},
	})

	_, _, err := aheadBehindCount(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error when rev-list command fails")
	}
}

func TestAheadBehindCount_MalformedOutput(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "malformed-ahead-repo", true)

	withMockGitPush(t, map[string]mockPushScenario{
		"malformed-ahead-repo": {
			revListOutput: "onlyone\n",
			revListExit:   0,
		},
	})

	_, _, err := aheadBehindCount(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error for malformed rev-list output")
	}
	if !strings.Contains(err.Error(), "unexpected rev-list output") {
		t.Errorf("expected 'unexpected rev-list output', got: %v", err)
	}
}

func TestAheadBehindCount_NonNumericFields(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "nonnumeric-ahead-repo", true)

	withMockGitPush(t, map[string]mockPushScenario{
		"nonnumeric-ahead-repo": {
			revListOutput: "abc\tdef\n",
			revListExit:   0,
		},
	})

	_, _, err := aheadBehindCount(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error for non-numeric rev-list output")
	}
	if !strings.Contains(err.Error(), "failed to parse") {
		t.Errorf("expected 'failed to parse' in error, got: %v", err)
	}
}

func TestPushRepo_ValidateDestinationError(t *testing.T) {
	base := t.TempDir()
	// A file (not a directory) will cause ValidateDestination to return an error.
	repoPath := filepath.Join(base, "file-not-dir")
	if err := os.WriteFile(repoPath, []byte("not a dir"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	_, _, err := pushRepo(context.Background(), repoPath)
	if err == nil {
		t.Fatal("expected an error when destination is a file, not a git repo")
	}
}

func TestPushRepo_SetUpstreamPushFails(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "set-upstream-fail-repo", true)

	withMockGitPush(t, map[string]mockPushScenario{
		"set-upstream-fail-repo": {
			branchOutput: "feature/new\n",
			upstreamExit: 128, // no upstream
			pushOutput:   "remote: push rejected",
			pushExit:     1,
		},
	})

	_, _, err := pushRepo(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error when --set-upstream push fails")
	}
}

func TestPushRepo_AheadBehindCountError(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "ahead-err-repo", true)

	withMockGitPush(t, map[string]mockPushScenario{
		"ahead-err-repo": {
			branchOutput:  "main\n",
			upstreamExit:  0,  // has upstream
			revListOutput: "",
			revListExit:   1, // rev-list fails
		},
	})

	_, _, err := pushRepo(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error when aheadBehindCount fails inside pushRepo")
	}
}

func TestCurrentBranch_CommandFails(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "branch-fail-repo", true)

	withMockGitPush(t, map[string]mockPushScenario{
		"branch-fail-repo": {
			branchOutput: "",
			branchExit:   1,
		},
	})

	_, err := currentBranch(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error when rev-parse command exits non-zero")
	}
}

func TestPushRepo_CurrentBranchError(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "branch-err-push-repo", true)

	withMockGitPush(t, map[string]mockPushScenario{
		"branch-err-push-repo": {
			branchOutput: "",
			branchExit:   1,
		},
	})

	_, _, err := pushRepo(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error when currentBranch fails inside pushRepo")
	}
}

func TestHasUpstreamConfigured_Missing(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "no-upstream-check-repo", true)

	withMockGitPush(t, map[string]mockPushScenario{
		"no-upstream-check-repo": {
			upstreamExit: 128,
		},
	})

	has, err := hasUpstreamConfigured(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("expected hasUpstream=false when rev-parse exits 128")
	}
}
