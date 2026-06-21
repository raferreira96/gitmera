package ui

import (
	"context"
	"fmt"
	"time"

	"gitmera/pkg/runner"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// spinnerFrames holds the animated dots-style spinner frames (D-01, D-03).
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// repoProgress tracks the rendering state of a single repository row in the
// interactive TUI list.
type repoProgress struct {
	name    string
	action  string
	status  string // "pending", "active", "success", "failed", "skipped"
	message string
	err     error
}

// spinnerTickMsg is sent on every spinner animation tick.
type spinnerTickMsg struct{}

// tickSpinner schedules the next spinner animation frame.
func tickSpinner() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

// eventClosedMsg signals that the upstream event channel has been closed
// (all tasks have reported a terminal event and the producer goroutine has
// finished), telling the TUI it is safe to render the final summary and
// quit.
type eventClosedMsg struct{}

// waitForEvent returns a tea.Cmd that blocks on the background event
// channel and yields the next runner.TaskEvent as a tea.Msg, or an
// eventClosedMsg once the channel is closed and drained.
func waitForEvent(ch <-chan runner.TaskEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return eventClosedMsg{}
		}
		return event
	}
}

// TUIModel implements the Bubble Tea v2 Elm-architecture Model for
// Gitmera's interactive concurrent-task progress view (D-01, D-02, D-03).
type TUIModel struct {
	tasks        []repoProgress
	taskMap      map[string]int
	spinnerFrame int
	eventChan    <-chan runner.TaskEvent
	cancel       context.CancelFunc
	quitting     bool
	closed       bool
}

// NewTUIModel constructs a TUIModel pre-populated with every repository
// task in "pending" state (D-02), ready to consume lifecycle events from
// eventChan. cancel is invoked when the user interrupts the TUI via Ctrl+C
// (D-12), allowing the caller to cancel the underlying runner context.
func NewTUIModel(tasks []runner.RepoTask, action string, eventChan <-chan runner.TaskEvent, cancel context.CancelFunc) TUIModel {
	progress := make([]repoProgress, len(tasks))
	taskMap := make(map[string]int, len(tasks))
	for i, t := range tasks {
		progress[i] = repoProgress{name: t.Name, action: action, status: "pending"}
		taskMap[t.Name] = i
	}
	return TUIModel{
		tasks:     progress,
		taskMap:   taskMap,
		eventChan: eventChan,
		cancel:    cancel,
	}
}

// Init starts the background event consumer loop and the spinner ticker.
func (m TUIModel) Init() tea.Cmd {
	return tea.Batch(waitForEvent(m.eventChan), tickSpinner())
}

// Update handles model mutations in a single, strictly sequential thread
// (the Bubble Tea Elm loop), guaranteeing race-free state transitions even
// though events originate from concurrent runner goroutines (D-09, D-10).
func (m TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case runner.TaskEvent:
		if idx, found := m.taskMap[msg.RepoName]; found {
			switch msg.Type {
			case runner.TaskStarted:
				m.tasks[idx].status = "active"
			case runner.TaskFinished:
				if msg.Err != nil {
					m.tasks[idx].status = "failed"
					m.tasks[idx].err = msg.Err
				} else {
					m.tasks[idx].status = "success"
				}
			case runner.TaskSkipped:
				m.tasks[idx].status = "skipped"
				m.tasks[idx].err = msg.Err
				m.tasks[idx].message = msg.Stderr
			}
		}
		if m.closed {
			return m, nil
		}
		return m, waitForEvent(m.eventChan)

	case eventClosedMsg:
		m.closed = true
		m.quitting = true
		return m, tea.Quit

	case spinnerTickMsg:
		if m.quitting {
			return m, nil
		}
		m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
		return m, tickSpinner()

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			if m.cancel != nil {
				m.cancel()
			}
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// Styling primitives for the TUI list view (D-03).
var (
	tuiPendingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))  // gray
	tuiActiveSpin   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))  // cyan
	tuiSuccessMark  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))  // green
	tuiFailedMark   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))  // red
	tuiWarnMark     = lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // yellow
	tuiRepoNameBold = lipgloss.NewStyle().Bold(true)
)

// View renders the current state of every repository task as a single
// list, using Lip Gloss styles to color-code pending/active/success/
// failed/skipped rows (D-01, D-02, D-03, D-13, D-14).
func (m TUIModel) View() tea.View {
	var s string
	for _, task := range m.tasks {
		name := tuiRepoNameBold.Render(task.name)
		switch task.status {
		case "pending":
			s += tuiPendingStyle.Render("  "+task.name+": pending") + "\n"
		case "active":
			spinner := tuiActiveSpin.Render(spinnerFrames[m.spinnerFrame])
			s += fmt.Sprintf("%s %s: %s...\n", spinner, name, task.action)
		case "success":
			chk := tuiSuccessMark.Render("✓")
			s += fmt.Sprintf("%s %s: success\n", chk, name)
		case "failed":
			cross := tuiFailedMark.Render("✗")
			s += fmt.Sprintf("%s %s: failed\n", cross, name)
		case "skipped":
			warn := tuiWarnMark.Render("⚠")
			if task.err != nil {
				s += fmt.Sprintf("%s %s: cancelled\n", warn, name)
			} else {
				s += fmt.Sprintf("%s %s: skipped\n", warn, name)
			}
		}
	}
	return tea.NewView(s)
}

// RunSequentialFallback drains eventChan sequentially and prints clean,
// chronological line-by-line status messages via logger, conforming to the
// non-TTY/CI fallback behavior (D-05, D-08). Colors are automatically
// stripped by SafeLogger when --no-color is set or stdout is not a
// terminal.
func RunSequentialFallback(eventChan <-chan runner.TaskEvent, logger *SafeLogger) {
	for event := range eventChan {
		repoStyle := lipgloss.NewStyle().Bold(true).Render(event.RepoName)
		switch event.Type {
		case runner.TaskStarted:
			logger.Print(fmt.Sprintf("[-] %s: started\n", repoStyle))
		case runner.TaskFinished:
			if event.Err != nil {
				cross := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("✗")
				logger.Print(fmt.Sprintf("[%s] %s: failed\n", cross, repoStyle))
			} else {
				chk := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("✓")
				logger.Print(fmt.Sprintf("[%s] %s: success\n", chk, repoStyle))
			}
		case runner.TaskSkipped:
			warn := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("⚠")
			if event.Err != nil {
				logger.Print(fmt.Sprintf("[%s] %s: cancelled\n", warn, repoStyle))
			} else {
				logger.Print(fmt.Sprintf("[%s] %s: skipped\n", warn, repoStyle))
			}
		}
	}
}

// ExecutionOptions configures OrchestrateExecution's behavior.
type ExecutionOptions struct {
	// Interactive selects whether to run the Bubble Tea TUI (true) or the
	// sequential fallback logger (false). Callers determine this from TTY
	// detection and the --non-interactive/--plain flags (D-07).
	Interactive bool
	// ActionLabel is the human-readable verb shown next to each active
	// repository row in the TUI (e.g. "cloning", "pulling").
	ActionLabel string
	// Logger receives sequential fallback output (D-05, D-06). Required
	// when Interactive is false; unused when Interactive is true.
	Logger *SafeLogger
}

// OrchestrateExecution is the unified entry point that forks between the
// interactive Bubble Tea TUI and the non-interactive sequential fallback,
// runs the concurrent runner.ExecuteTasks call in a background goroutine
// fed by a properly buffered event channel, and returns the final
// per-repository results once execution completes (D-09, D-10).
//
// ctx is the caller's execution context; OrchestrateExecution derives an
// internal cancellable context so that a Ctrl+C captured by the TUI can
// terminate the runner and its Git subprocesses cleanly before the
// function returns (D-12).
func OrchestrateExecution(
	ctx context.Context,
	tasks []runner.RepoTask,
	concurrency int,
	failFast bool,
	taskTimeout time.Duration,
	action runner.TaskActionFunc,
	opts ExecutionOptions,
) []runner.TaskResult {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Buffer size guarantee: at least len(tasks)*2 so workers never block on
	// channel writes even if the consumer is slow or exits early.
	bufSize := len(tasks) * 2
	if bufSize < 1 {
		bufSize = 1
	}
	eventChan := make(chan runner.TaskEvent, bufSize)

	resultsChan := make(chan []runner.TaskResult, 1)
	go func() {
		results := runner.ExecuteTasks(runCtx, tasks, concurrency, failFast, taskTimeout, action, eventChan)
		close(eventChan)
		resultsChan <- results
	}()

	if opts.Interactive {
		model := NewTUIModel(tasks, opts.ActionLabel, eventChan, cancel)
		p := tea.NewProgram(model)
		_, _ = p.Run()
		DrainStdinNonblocking() // drain orphaned PTY responses (e.g. DECRQM) before TCFLSH
		FlushStdin()
	} else {
		RunSequentialFallback(eventChan, opts.Logger)
	}

	return <-resultsChan
}
