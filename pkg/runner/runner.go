// Package runner orchestrates concurrent execution of repository actions (clone, pull)
// using bounded concurrency, execution policies (fail-fast vs keep-going), and double-context timeouts.
package runner

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// RepoTask represents a single repository target and the action to perform on it.
type RepoTask struct {
	Name string
	URI  string
	Path string
	// Action is the verb being performed: one of "clone", "pull", "push",
	// "checkout", or "status". It is informational only — the runner does
	// not branch on it; the caller-supplied TaskActionFunc does.
	Action string
}

// TaskResult holds the execution outcome for a specific repository.
type TaskResult struct {
	RepoName string
	Err      error
	Stderr   string
	Skipped  bool
}

// TaskActionFunc represents the callback executing the actual Git operation on a repository.
type TaskActionFunc func(ctx context.Context, task RepoTask) (err error, stderr string, skipped bool)

// TaskEventType identifies the kind of lifecycle event emitted for a
// repository task during concurrent execution.
type TaskEventType string

const (
	// TaskStarted is emitted immediately before a worker invokes the action
	// callback for a repository task.
	TaskStarted TaskEventType = "started"
	// TaskFinished is emitted after the action callback completes
	// (successfully or with an error), excluding skipped outcomes.
	TaskFinished TaskEventType = "finished"
	// TaskSkipped is emitted when a task is skipped, either by the action
	// callback's own logic (e.g. already cloned) or because the task was
	// cancelled before running due to a sibling failure under fail-fast.
	TaskSkipped TaskEventType = "skipped"
)

// TaskEvent represents a single lifecycle update for a repository task,
// dispatched on the optional event channel passed to ExecuteTasks so
// consumers (e.g. a Bubble Tea TUI or a sequential fallback logger) can
// render real-time progress without being coupled to runner internals.
type TaskEvent struct {
	RepoName string
	Type     TaskEventType
	Err      error
	Stderr   string
	Skipped  bool
}

// ExecuteTasks runs Git actions concurrently with a worker limit.
// It manages bounds via errgroup, individual task timeouts, and keep-going/fail-fast policies.
//
// eventChan is an optional write-only channel used to broadcast TaskStarted,
// TaskFinished, and TaskSkipped lifecycle events as tasks progress. Pass nil
// to preserve the previous (event-less) behavior. Callers that do supply a
// channel MUST buffer it with a capacity of at least len(tasks)*2 so workers
// never block on a slow or absent consumer.
func ExecuteTasks(
	ctx context.Context,
	tasks []RepoTask,
	concurrency int,
	failFast bool,
	taskTimeout time.Duration,
	action TaskActionFunc,
	eventChan chan<- TaskEvent,
) []TaskResult {
	results := make([]TaskResult, len(tasks))
	var resultsMu sync.Mutex

	// Bounded worker group
	var g errgroup.Group
	g.SetLimit(concurrency)

	// Create a cancellable group context for fail-fast behaviors
	groupCtx, cancelGroup := context.WithCancel(ctx)
	defer cancelGroup()

	for i, task := range tasks {
		i, task := i, task
		g.Go(func() error {
			// Early exit if context was cancelled by another task's failure
			if groupCtx.Err() != nil {
				res := TaskResult{RepoName: task.Name, Err: groupCtx.Err(), Skipped: true}
				resultsMu.Lock()
				results[i] = res
				resultsMu.Unlock()
				if eventChan != nil {
					eventChan <- TaskEvent{
						RepoName: task.Name,
						Type:     TaskSkipped,
						Err:      groupCtx.Err(),
						Skipped:  true,
					}
				}
				return groupCtx.Err()
			}

			// Individual worker timeout context
			workerCtx, workerCancel := context.WithTimeout(groupCtx, taskTimeout)
			defer workerCancel()

			if eventChan != nil {
				eventChan <- TaskEvent{RepoName: task.Name, Type: TaskStarted}
			}

			// Execute the action callback
			err, stderr, skipped := action(workerCtx, task)

			res := TaskResult{
				RepoName: task.Name,
				Err:      err,
				Stderr:   stderr,
				Skipped:  skipped,
			}

			resultsMu.Lock()
			results[i] = res
			resultsMu.Unlock()

			if eventChan != nil {
				if skipped {
					eventChan <- TaskEvent{
						RepoName: task.Name,
						Type:     TaskSkipped,
						Err:      err,
						Stderr:   stderr,
						Skipped:  true,
					}
				} else {
					eventChan <- TaskEvent{
						RepoName: task.Name,
						Type:     TaskFinished,
						Err:      err,
						Stderr:   stderr,
					}
				}
			}

			if err != nil && failFast {
				cancelGroup() // Cancel other running/pending tasks
				return err
			}

			return nil
		})
	}

	// Wait for all workers to return or cancel
	_ = g.Wait()

	// Fill in any results that were not written due to early cancellation,
	// emitting a TaskSkipped event for each so consumers see a terminal
	// event for every task even when it never reached the worker body.
	resultsMu.Lock()
	for i, task := range tasks {
		if results[i].RepoName == "" {
			results[i] = TaskResult{
				RepoName: task.Name,
				Err:      groupCtx.Err(),
				Skipped:  true,
			}
			if eventChan != nil {
				eventChan <- TaskEvent{
					RepoName: task.Name,
					Type:     TaskSkipped,
					Err:      groupCtx.Err(),
					Skipped:  true,
				}
			}
		}
	}
	resultsMu.Unlock()

	return results
}
