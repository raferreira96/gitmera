package cmd

import (
	"bytes"
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

	"charm.land/lipgloss/v2"
)

// mockStatusScenario describes the simulated git status/rev-list output for
// a single repository directory during a mocked status collection run.
type mockStatusScenario struct {
	// statusOutput is the raw stdout/stderr returned for `git status
	// --porcelain=v1 -b`.
	statusOutput string
	statusExit   int

	// revListOutput is the raw stdout for `git rev-list --count
	// --left-right HEAD...@{u}`.
	revListOutput string
	revListExit   int
}

// withMockGitStatus installs a subprocess mock keyed by repository
// directory basename, routing `git` invocations to a re-exec'd
// TestStatusHelperProcess. Because the helper runs as a genuinely separate
// OS process (not a goroutine), scenario data cannot be shared via package
// variables; instead it is base64-encoded and passed through environment
// variables, looked up by the basename of the working directory the mocked
// command is invoked in (git.RunGitCommand sets cmd.Dir to the repo path).
// The restore is registered via t.Cleanup.
func withMockGitStatus(t *testing.T, scenarios map[string]mockStatusScenario) {
	t.Helper()

	restore := git.SetExecCommandForTest(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestStatusHelperProcess", "--"}
		cs = append(cs, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		env := append(os.Environ(), "GO_WANT_STATUS_HELPER_PROCESS=1")
		for repoName, sc := range scenarios {
			prefix := "GITMERA_MOCK_" + sanitizeEnvKey(repoName) + "_"
			env = append(env,
				prefix+"STATUS_OUT="+base64.StdEncoding.EncodeToString([]byte(sc.statusOutput)),
				prefix+"STATUS_EXIT="+strconv.Itoa(sc.statusExit),
				prefix+"REVLIST_OUT="+base64.StdEncoding.EncodeToString([]byte(sc.revListOutput)),
				prefix+"REVLIST_EXIT="+strconv.Itoa(sc.revListExit),
			)
		}
		cmd.Env = env
		return cmd
	})
	t.Cleanup(restore)
}

// sanitizeEnvKey upper-cases repoName and replaces characters that are not
// valid in a portable environment variable name (env var names are
// restricted to [A-Za-z0-9_]) with underscores, so repo names containing
// hyphens (a common, valid directory-name character) can still be encoded
// as a lookup key.
func sanitizeEnvKey(repoName string) string {
	var sb strings.Builder
	for _, r := range strings.ToUpper(repoName) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	return sb.String()
}

// TestStatusHelperProcess is the subprocess entry point used by
// withMockGitStatus. It inspects the current working directory (set via
// exec.Cmd.Dir by git.RunGitCommand) to select the right scenario, then
// emits the configured mock output for the requested git subcommand.
func TestStatusHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_STATUS_HELPER_PROCESS") != "1" {
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
	case "fetch":
		os.Exit(0)
	case "status":
		out, err := base64.StdEncoding.DecodeString(os.Getenv(prefix + "STATUS_OUT"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "no scenario configured for repo %q\n", repoName)
			os.Exit(1)
		}
		exitCode, _ := strconv.Atoi(os.Getenv(prefix + "STATUS_EXIT"))
		fmt.Print(string(out))
		os.Exit(exitCode)
	case "rev-list":
		out, err := base64.StdEncoding.DecodeString(os.Getenv(prefix + "REVLIST_OUT"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "no scenario configured for repo %q\n", repoName)
			os.Exit(1)
		}
		exitCode, _ := strconv.Atoi(os.Getenv(prefix + "REVLIST_EXIT"))
		fmt.Print(string(out))
		os.Exit(exitCode)
	default:
		fmt.Fprintf(os.Stderr, "unexpected git subcommand %q\n", subCmd)
		os.Exit(1)
	}
}

// makeRepoDir creates a temp directory (optionally containing a .git
// subdirectory to satisfy git.ValidateDestination) named after repoName,
// returning its full path.
func makeRepoDir(t *testing.T, base, repoName string, withGitDir bool) string {
	t.Helper()
	dir := filepath.Join(base, repoName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}
	if withGitDir {
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
			t.Fatalf("failed to create .git dir: %v", err)
		}
	}
	return dir
}

func TestCollectRepoStatus_Clean(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "clean-repo", true)

	withMockGitStatus(t, map[string]mockStatusScenario{
		"clean-repo": {
			statusOutput:  "## main...origin/main\n",
			revListOutput: "0\t0\n",
		},
	})

	rs := collectRepoStatus(context.Background(), "clean-repo", dir, false)

	if rs.Status != "Clean" {
		t.Errorf("expected Status=Clean, got %q", rs.Status)
	}
	if rs.Branch != "main" {
		t.Errorf("expected Branch=main, got %q", rs.Branch)
	}
	if !rs.HasUpstream {
		t.Error("expected HasUpstream=true")
	}
	if !rs.UpToDate {
		t.Error("expected UpToDate=true")
	}
}

func TestCollectRepoStatus_ModifiedWithUntracked(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "modified-repo", true)

	withMockGitStatus(t, map[string]mockStatusScenario{
		"modified-repo": {
			statusOutput:  "## main...origin/main\n M main.go\n?? untracked_file.go\n",
			revListOutput: "0\t0\n",
		},
	})

	rs := collectRepoStatus(context.Background(), "modified-repo", dir, false)

	if rs.Status != "Modified" {
		t.Errorf("expected Status=Modified (untracked files count as dirty per D-04), got %q", rs.Status)
	}
}

func TestCollectRepoStatus_Missing(t *testing.T) {
	base := t.TempDir()
	// Directory does not exist at all.
	dir := filepath.Join(base, "missing-repo")

	rs := collectRepoStatus(context.Background(), "missing-repo", dir, false)

	if rs.Status != "Missing" {
		t.Errorf("expected Status=Missing, got %q", rs.Status)
	}
}

func TestCollectRepoStatus_Ahead(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "ahead-repo", true)

	withMockGitStatus(t, map[string]mockStatusScenario{
		"ahead-repo": {
			statusOutput:  "## main...origin/main [ahead 2]\n",
			revListOutput: "2\t0\n",
		},
	})

	rs := collectRepoStatus(context.Background(), "ahead-repo", dir, false)

	if rs.Ahead != 2 || rs.Behind != 0 {
		t.Errorf("expected ahead=2 behind=0, got ahead=%d behind=%d", rs.Ahead, rs.Behind)
	}
	if rs.Diverged {
		t.Error("expected Diverged=false")
	}
}

func TestCollectRepoStatus_Behind(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "behind-repo", true)

	withMockGitStatus(t, map[string]mockStatusScenario{
		"behind-repo": {
			statusOutput:  "## main...origin/main [behind 1]\n",
			revListOutput: "0\t1\n",
		},
	})

	rs := collectRepoStatus(context.Background(), "behind-repo", dir, false)

	if rs.Ahead != 0 || rs.Behind != 1 {
		t.Errorf("expected ahead=0 behind=1, got ahead=%d behind=%d", rs.Ahead, rs.Behind)
	}
}

func TestCollectRepoStatus_Diverged(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "diverged-repo", true)

	withMockGitStatus(t, map[string]mockStatusScenario{
		"diverged-repo": {
			statusOutput:  "## main...origin/main [ahead 3, behind 4]\n",
			revListOutput: "3\t4\n",
		},
	})

	rs := collectRepoStatus(context.Background(), "diverged-repo", dir, false)

	if !rs.Diverged {
		t.Error("expected Diverged=true")
	}
	if rs.Ahead != 3 || rs.Behind != 4 {
		t.Errorf("expected ahead=3 behind=4, got ahead=%d behind=%d", rs.Ahead, rs.Behind)
	}
}

func TestCollectRepoStatus_NoUpstream(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "no-upstream-repo", true)

	withMockGitStatus(t, map[string]mockStatusScenario{
		"no-upstream-repo": {
			statusOutput: "## feature-branch\n",
		},
	})

	rs := collectRepoStatus(context.Background(), "no-upstream-repo", dir, false)

	if rs.HasUpstream {
		t.Error("expected HasUpstream=false")
	}
	if rs.Branch != "feature-branch" {
		t.Errorf("expected Branch=feature-branch, got %q", rs.Branch)
	}
}

func TestCollectRepoStatus_DetachedHead(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "detached-repo", true)

	withMockGitStatus(t, map[string]mockStatusScenario{
		"detached-repo": {
			statusOutput: "## HEAD (no branch)\n",
		},
	})

	rs := collectRepoStatus(context.Background(), "detached-repo", dir, false)

	if !rs.Detached {
		t.Error("expected Detached=true")
	}
	if rs.Branch != "(detached HEAD)" {
		t.Errorf("expected Branch=(detached HEAD), got %q", rs.Branch)
	}
}

func TestCollectRepoStatus_RevListFallbackToNoUpstream(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "fallback-repo", true)

	withMockGitStatus(t, map[string]mockStatusScenario{
		"fallback-repo": {
			statusOutput: "## main...origin/main\n",
			revListExit:  128,
		},
	})

	rs := collectRepoStatus(context.Background(), "fallback-repo", dir, false)

	if rs.HasUpstream {
		t.Error("expected HasUpstream=false after rev-list failure fallback")
	}
}

func TestCollectRepoStatus_FetchInvoked(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "fetch-repo", true)

	withMockGitStatus(t, map[string]mockStatusScenario{
		"fetch-repo": {
			statusOutput:  "## main...origin/main\n",
			revListOutput: "0\t0\n",
		},
	})

	// doFetch=true must not break collection; the mock's "fetch" case
	// simply exits 0 without affecting subsequent status/rev-list calls.
	rs := collectRepoStatus(context.Background(), "fetch-repo", dir, true)

	if rs.Status != "Clean" {
		t.Errorf("expected Status=Clean after fetch+status, got %q", rs.Status)
	}
}

func TestRenderStatusTable_Alignment(t *testing.T) {
	statuses := []repoStatus{
		{Name: "short", Branch: "main", Status: "Clean", HasUpstream: true, UpToDate: true},
		{Name: "a-much-longer-repo-name", Branch: "feature/long-branch-name", Status: "Modified", HasUpstream: true, Ahead: 1, Behind: 0},
		{Name: "missing-one", Status: "Missing"},
	}

	var buf bytes.Buffer
	renderStatusTable(&buf, statuses)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != len(statuses)+1 {
		t.Fatalf("expected %d lines (header + rows), got %d", len(statuses)+1, len(lines))
	}

	for i, line := range lines {
		if line == "" {
			t.Errorf("line %d is empty, expected rendered content", i)
		}
	}
}

func TestRenderMissingDetails_SurfacesUnderlyingError(t *testing.T) {
	statuses := []repoStatus{
		{Name: "clean-repo", Status: "Clean"},
		{Name: "broken-repo", Status: "Missing", Err: fmt.Errorf("destination path %q exists but is not a valid Git repository (missing .git directory)", "/tmp/broken-repo")},
		{Name: "plain-missing", Status: "Missing"}, // Err is nil: path simply doesn't exist
	}

	var buf bytes.Buffer
	renderMissingDetails(&buf, statuses)

	out := buf.String()
	if !strings.Contains(out, "broken-repo") || !strings.Contains(out, "missing .git directory") {
		t.Errorf("expected a detail line surfacing broken-repo's specific error, got %q", out)
	}
	if strings.Contains(out, "plain-missing") {
		t.Errorf("expected no detail line for a Missing repo with a nil Err, got %q", out)
	}
	if strings.Contains(out, "clean-repo") {
		t.Errorf("expected no detail line for a Clean repo, got %q", out)
	}
}

func TestCollectRepoStatus_GitStatusFailure(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "fail-status-repo", true)

	withMockGitStatus(t, map[string]mockStatusScenario{
		"fail-status-repo": {
			statusOutput: "",
			statusExit:   128,
		},
	})

	rs := collectRepoStatus(context.Background(), "fail-status-repo", dir, false)

	if rs.Status != "Missing" {
		t.Errorf("expected Status=Missing when git status fails, got %q", rs.Status)
	}
	if rs.Err == nil {
		t.Error("expected non-nil Err when git status fails")
	}
}

func TestCollectRepoStatus_UnexpectedStatusFormat(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "weird-format-repo", true)

	withMockGitStatus(t, map[string]mockStatusScenario{
		"weird-format-repo": {
			statusOutput: "not a porcelain header\n",
			statusExit:   0,
		},
	})

	rs := collectRepoStatus(context.Background(), "weird-format-repo", dir, false)

	if rs.Status != "Missing" {
		t.Errorf("expected Status=Missing for unexpected output format, got %q", rs.Status)
	}
	if rs.Err == nil {
		t.Error("expected non-nil Err for missing branch header")
	}
}

func TestParseBranchLine_NoCommitsYet(t *testing.T) {
	var rs repoStatus
	parseBranchLine("## No commits yet on main", &rs)

	if rs.Branch != "main" {
		t.Errorf("expected branch 'main', got %q", rs.Branch)
	}
	if rs.HasUpstream {
		t.Error("expected HasUpstream=false for no-commits branch")
	}
	if rs.Detached {
		t.Error("expected Detached=false for no-commits branch")
	}
}

func TestComputeAheadBehind_MalformedFieldCount(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "malformed-revlist-repo", true)

	withMockGitStatus(t, map[string]mockStatusScenario{
		"malformed-revlist-repo": {
			statusOutput:  "## main...origin/main\n",
			revListOutput: "singlevalue\n",
			revListExit:   0,
		},
	})

	rs := collectRepoStatus(context.Background(), "malformed-revlist-repo", dir, false)

	if rs.HasUpstream {
		t.Error("expected HasUpstream=false after malformed rev-list output (wrong field count)")
	}
}

func TestComputeAheadBehind_NonNumericFields(t *testing.T) {
	base := t.TempDir()
	dir := makeRepoDir(t, base, "nonnumeric-revlist-repo", true)

	withMockGitStatus(t, map[string]mockStatusScenario{
		"nonnumeric-revlist-repo": {
			statusOutput:  "## main...origin/main\n",
			revListOutput: "abc\tdef\n",
			revListExit:   0,
		},
	})

	rs := collectRepoStatus(context.Background(), "nonnumeric-revlist-repo", dir, false)

	if rs.HasUpstream {
		t.Error("expected HasUpstream=false after non-numeric rev-list output")
	}
}

func TestRenderStatusCell_DefaultCase(t *testing.T) {
	rs := repoStatus{Name: "odd", Status: "unknown-state"}
	got := renderStatusCell(rs)
	if got == "" {
		t.Error("expected non-empty output for default status cell")
	}
	if !strings.Contains(got, "unknown-state") {
		t.Errorf("expected output to contain the status value, got: %q", got)
	}
}

func TestRenderAheadBehindCell_NoUpstream(t *testing.T) {
	rs := repoStatus{Name: "no-upstream-repo", Branch: "main", Status: "Clean", HasUpstream: false}
	got := renderAheadBehindCell(rs)
	if !strings.Contains(got, "no upstream") {
		t.Errorf("expected 'no upstream', got: %q", got)
	}
}

func TestRenderAheadBehindCell_Diverged(t *testing.T) {
	rs := repoStatus{
		Name: "diverged", Status: "Clean", HasUpstream: true,
		Ahead: 2, Behind: 3, Diverged: true,
	}
	got := renderAheadBehindCell(rs)
	if !strings.Contains(got, "diverged") {
		t.Errorf("expected 'diverged', got: %q", got)
	}
}

func TestRenderAheadBehindCell_Behind(t *testing.T) {
	rs := repoStatus{
		Name: "behind", Status: "Clean", HasUpstream: true,
		Ahead: 0, Behind: 2, Diverged: false, UpToDate: false,
	}
	got := renderAheadBehindCell(rs)
	if !strings.Contains(got, "behind") {
		t.Errorf("expected 'behind', got: %q", got)
	}
}

func TestRenderAheadBehindCell_UpToDate(t *testing.T) {
	rs := repoStatus{
		Name: "sync", Status: "Clean", HasUpstream: true,
		Ahead: 0, Behind: 0, Diverged: false, UpToDate: true,
	}
	got := renderAheadBehindCell(rs)
	if !strings.Contains(got, "up-to-date") {
		t.Errorf("expected 'up-to-date', got: %q", got)
	}
}

func TestPadRight_UsesVisualWidth(t *testing.T) {
	// Confirms padRight measures visual (ANSI-aware) width via
	// lipgloss.Width rather than raw byte length, so styled strings still
	// align correctly when interleaved with plain strings in a column.
	got := padRight("ab", 5)
	if len(got) != 5 {
		t.Errorf("expected padded plain string of length 5, got %q (%d)", got, len(got))
	}

	styled := statusCleanStyle.Render("ab")
	gotStyled := padRight(styled, 5)
	if w := lipgloss.Width(gotStyled); w != 5 {
		t.Errorf("expected visual width 5 for padded styled string, got %d (%q)", w, gotStyled)
	}
}
