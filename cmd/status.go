package cmd

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitmera/pkg/git"
	"gitmera/pkg/runner"
	"gitmera/pkg/ui"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
)

var (
	statusConcurrency int
	statusTimeout     time.Duration
	statusFetch       bool
)

// repoStatus represents the collected, parsed status of a single child
// repository, ready to be rendered as a row in the status table.
type repoStatus struct {
	Name        string
	Branch      string
	Detached    bool
	Status      string // "Clean", "Modified", "Missing"
	Ahead       int
	Behind      int
	HasUpstream bool
	Diverged    bool // ahead > 0 && behind > 0
	UpToDate    bool // ahead == 0 && behind == 0 (with upstream)
	Err         error
}

var statusCmd = &cobra.Command{
	Use:          "status",
	Short:        "Show a unified status summary across all configured repositories",
	Long:         `Reads the workspace configuration and concurrently checks git status in all child repositories, displaying a colorized table summarizing branch, modification, and ahead/behind divergence state.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		setup, err := setupCommand(cmd, statusConcurrency, statusTimeout, "status")
		if err != nil {
			return err
		}
		defer setup.cancel()

		statuses := make(map[string]repoStatus, len(setup.tasks))
		var statusMu sync.Mutex

		action := func(workerCtx context.Context, task runner.RepoTask) (error, string, bool) {
			rs := collectRepoStatus(workerCtx, task.Name, task.Path, statusFetch)
			statusMu.Lock()
			statuses[task.Name] = rs
			statusMu.Unlock()
			return nil, "", false
		}

		// Status always uses the keep-going policy: every repository should
		// be inspected regardless of individual failures (fail-fast is
		// never appropriate for a read-only summary command).
		runner.ExecuteTasks(setup.ctx, setup.tasks, setup.concurrency, false, setup.timeout, action, nil)

		// Render results in the same sorted order as setup.tasks.
		ordered := make([]repoStatus, 0, len(setup.tasks))
		for _, task := range setup.tasks {
			ordered = append(ordered, statuses[task.Name])
		}

		var tableBuf strings.Builder
		renderStatusTable(&tableBuf, ordered)
		renderMissingDetails(&tableBuf, ordered)

		// Route through SafeLogger so --no-color and non-TTY output
		// streams are honored consistently with clone/pull/init (it
		// downgrades/strips ANSI styling automatically when appropriate).
		logger := ui.NewSafeLogger(cmd.OutOrStdout(), noColor)
		logger.Print(tableBuf.String())

		return nil
	},
}

func init() {
	statusCmd.Flags().IntVarP(&statusConcurrency, "concurrency", "j", 5, "Maximum number of concurrent status check operations")
	statusCmd.Flags().DurationVar(&statusTimeout, "timeout", 2*time.Minute, "Timeout for each individual status check operation")
	statusCmd.Flags().BoolVarP(&statusFetch, "fetch", "f", false, "Perform a concurrent git fetch in all repositories before computing status")
	rootCmd.AddCommand(statusCmd)
}

// collectRepoStatus inspects a single repository: validating its existence,
// optionally fetching from the remote, parsing the porcelain status output,
// and computing local ahead/behind divergence against the upstream branch.
func collectRepoStatus(ctx context.Context, name, path string, doFetch bool) repoStatus {
	rs := repoStatus{Name: name}

	valid, err := git.ValidateDestination(path)
	if err != nil || !valid {
		rs.Status = "Missing"
		rs.Err = err
		return rs
	}

	if doFetch {
		// Best-effort: fetch failures should not prevent status from being
		// computed locally; they are simply not surfaced as fatal errors.
		_, _ = git.RunGitCommand(ctx, path, "fetch")
	}

	output, err := git.RunGitCommand(ctx, path, "status", "--porcelain=v1", "-b")
	if err != nil {
		rs.Status = "Missing"
		rs.Err = err
		return rs
	}

	lines := strings.Split(strings.TrimRight(string(output), "\n"), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "## ") {
		rs.Status = "Missing"
		rs.Err = fmt.Errorf("unexpected git status output: missing branch header")
		return rs
	}

	parseBranchLine(lines[0], &rs)

	// Determine dirty/clean status from any subsequent porcelain lines.
	// Both tracked modifications and untracked files (`??`) count towards
	// the dirty/Modified status (D-04).
	if len(lines) > 1 {
		rs.Status = "Modified"
	} else {
		rs.Status = "Clean"
	}

	if rs.HasUpstream {
		computeAheadBehind(ctx, path, &rs)
	}

	return rs
}

// parseBranchLine parses the `## ...` header line emitted by
// `git status --porcelain=v1 -b` to extract the local branch name and
// detect whether an upstream/tracking branch is configured.
func parseBranchLine(line string, rs *repoStatus) {
	header := strings.TrimPrefix(line, "## ")

	if strings.HasPrefix(header, "HEAD (no branch)") {
		rs.Detached = true
		rs.Branch = "(detached HEAD)"
		rs.HasUpstream = false
		return
	}

	// Strip optional "[ahead X, behind Y]" suffix before tracking parsing.
	if idx := strings.Index(header, " ["); idx != -1 {
		header = header[:idx]
	}

	if strings.Contains(header, "...") {
		parts := strings.SplitN(header, "...", 2)
		rs.Branch = parts[0]
		rs.HasUpstream = true
		return
	}

	// No tracking branch. Handle the "No commits yet on <branch>" format
	// emitted by Git for freshly initialized, commit-less repositories.
	if strings.HasPrefix(header, "No commits yet on ") {
		rs.Branch = strings.TrimPrefix(header, "No commits yet on ")
	} else {
		rs.Branch = header
	}
	rs.HasUpstream = false
}

// computeAheadBehind runs `git rev-list --count --left-right HEAD...@{u}`
// to determine the exact local ahead/behind divergence count against the
// configured upstream branch, purely locally (D-02). If the command fails
// (e.g. exit code 128 due to a missing/invalid upstream ref), it falls back
// to reporting no upstream.
func computeAheadBehind(ctx context.Context, path string, rs *repoStatus) {
	output, err := git.RunGitCommand(ctx, path, "rev-list", "--count", "--left-right", "HEAD...@{u}")
	if err != nil {
		rs.HasUpstream = false
		return
	}

	fields := strings.Fields(strings.TrimSpace(string(output)))
	if len(fields) != 2 {
		rs.HasUpstream = false
		return
	}

	ahead, errA := strconv.Atoi(fields[0])
	behind, errB := strconv.Atoi(fields[1])
	if errA != nil || errB != nil {
		rs.HasUpstream = false
		return
	}

	rs.Ahead = ahead
	rs.Behind = behind
	rs.Diverged = ahead > 0 && behind > 0
	rs.UpToDate = ahead == 0 && behind == 0
}

// Table column styles (D-01).
var (
	statusRepoStyle     = lipgloss.NewStyle().Bold(true)
	statusBranchStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("6")) // cyan
	statusCleanStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	statusModifiedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	statusMissingStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	statusGrayStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // gray
	// Note: deliberately Bold-only (no Underline). Lip Gloss v2.0.4 renders
	// combined Bold+Underline (and Underline alone) by emitting a separate
	// ANSI escape sequence around *each individual character* instead of
	// wrapping the whole string once, which inflates output size and can
	// visually corrupt column alignment in some terminal emulators. Bold
	// alone renders as a single escape sequence and is sufficient to
	// visually distinguish header cells from data rows.
	statusHeaderStyle = lipgloss.NewStyle().Bold(true)
)

// renderStatusTable formats the collected repository statuses into an
// aligned, colorized Lip Gloss table and writes it to w. Column widths are
// measured with lipgloss.Width to avoid ANSI escape sequences skewing
// alignment calculations.
func renderStatusTable(w io.Writer, statuses []repoStatus) {
	headers := []string{"REPOSITORY", "BRANCH", "STATUS", "AHEAD/BEHIND"}

	rows := make([][]string, 0, len(statuses))
	for _, rs := range statuses {
		rows = append(rows, []string{
			statusRepoStyle.Render(rs.Name),
			renderBranchCell(rs),
			renderStatusCell(rs),
			renderAheadBehindCell(rs),
		})
	}

	// Compute column widths using visual (ANSI-aware) width, considering
	// both header text and every rendered cell in that column.
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = lipgloss.Width(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if cw := lipgloss.Width(cell); cw > widths[i] {
				widths[i] = cw
			}
		}
	}

	var sb strings.Builder

	// Header row
	for i, h := range headers {
		sb.WriteString(padRight(statusHeaderStyle.Render(h), widths[i]))
		if i < len(headers)-1 {
			sb.WriteString("  ")
		}
	}
	sb.WriteString("\n")

	// Data rows
	for _, row := range rows {
		for i, cell := range row {
			sb.WriteString(padRight(cell, widths[i]))
			if i < len(row)-1 {
				sb.WriteString("  ")
			}
		}
		sb.WriteString("\n")
	}

	fmt.Fprint(w, sb.String())
}

// renderMissingDetails writes a detail line for each Missing repository that
// carries a specific underlying error (e.g. exists but isn't a valid Git
// repository, or the git status command itself failed), surfacing the
// reason instead of silently dropping it. Repositories that are Missing
// simply because the path doesn't exist at all carry no Err and are skipped.
func renderMissingDetails(w io.Writer, statuses []repoStatus) {
	for _, rs := range statuses {
		if rs.Status == "Missing" && rs.Err != nil {
			fmt.Fprintf(w, "  ↳ %s: %s\n", rs.Name, rs.Err.Error())
		}
	}
}

// padRight right-pads str with spaces until it reaches the target visual
// width, using lipgloss.Width to correctly measure strings that may contain
// invisible ANSI styling escape sequences.
func padRight(str string, width int) string {
	visualWidth := lipgloss.Width(str)
	if visualWidth >= width {
		return str
	}
	return str + strings.Repeat(" ", width-visualWidth)
}

// renderBranchCell renders the Branch column: cyan for both regular branch
// names and detached HEAD state.
func renderBranchCell(rs repoStatus) string {
	if rs.Status == "Missing" {
		return statusGrayStyle.Render("-")
	}
	return statusBranchStyle.Render(rs.Branch)
}

// renderStatusCell renders the Status column: green Clean, yellow Modified,
// red Missing.
func renderStatusCell(rs repoStatus) string {
	switch rs.Status {
	case "Clean":
		return statusCleanStyle.Render("Clean")
	case "Modified":
		return statusModifiedStyle.Render("Modified")
	case "Missing":
		return statusMissingStyle.Render("Missing")
	default:
		return statusGrayStyle.Render(rs.Status)
	}
}

// renderAheadBehindCell renders the Ahead/Behind column according to the
// divergence state: gray up-to-date/no-upstream, green ahead, yellow
// behind, red diverged.
func renderAheadBehindCell(rs repoStatus) string {
	if rs.Status == "Missing" {
		return statusGrayStyle.Render("-")
	}
	if !rs.HasUpstream {
		return statusGrayStyle.Render("no upstream")
	}
	switch {
	case rs.Diverged:
		return statusMissingStyle.Render(fmt.Sprintf("diverged (ahead %d, behind %d)", rs.Ahead, rs.Behind))
	case rs.Ahead > 0:
		return statusCleanStyle.Render(fmt.Sprintf("ahead %d", rs.Ahead))
	case rs.Behind > 0:
		return statusModifiedStyle.Render(fmt.Sprintf("behind %d", rs.Behind))
	default:
		return statusGrayStyle.Render("up-to-date")
	}
}
