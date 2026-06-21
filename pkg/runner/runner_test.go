package runner

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestExecuteTasks_ConcurrencyBounds(t *testing.T) {
	tasks := []RepoTask{
		{Name: "task1"},
		{Name: "task2"},
		{Name: "task3"},
		{Name: "task4"},
		{Name: "task5"},
	}

	var active int32
	var maxActive int32

	action := func(ctx context.Context, task RepoTask) (error, string, bool) {
		current := atomic.AddInt32(&active, 1)
		defer atomic.AddInt32(&active, -1)

		for {
			max := atomic.LoadInt32(&maxActive)
			if current <= max || atomic.CompareAndSwapInt32(&maxActive, max, current) {
				break
			}
		}

		time.Sleep(10 * time.Millisecond)
		return nil, "", false
	}

	concurrency := 2
	results := ExecuteTasks(context.Background(), tasks, concurrency, false, 1*time.Second, action, nil)

	if len(results) != len(tasks) {
		t.Fatalf("expected %d results, got %d", len(tasks), len(results))
	}

	finalMaxActive := atomic.LoadInt32(&maxActive)
	if finalMaxActive > int32(concurrency) {
		t.Errorf("expected max active workers to be <= %d, got %d", concurrency, finalMaxActive)
	}
}

func TestExecuteTasks_KeepGoing(t *testing.T) {
	tasks := []RepoTask{
		{Name: "success1"},
		{Name: "fail"},
		{Name: "success2"},
	}

	action := func(ctx context.Context, task RepoTask) (error, string, bool) {
		if task.Name == "fail" {
			return errors.New("failed task"), "stderr output", false
		}
		if task.Name == "success2" {
			return nil, "", true // simulate skipped
		}
		return nil, "", false
	}

	results := ExecuteTasks(context.Background(), tasks, 2, false, 1*time.Second, action, nil)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Verify all tasks ran and results were captured
	var foundFail, foundSkip, foundSuccess bool
	for _, res := range results {
		switch res.RepoName {
		case "fail":
			foundFail = true
			if res.Err == nil || res.Err.Error() != "failed task" {
				t.Errorf("expected error 'failed task', got %v", res.Err)
			}
			if res.Stderr != "stderr output" {
				t.Errorf("expected stderr 'stderr output', got %q", res.Stderr)
			}
		case "success2":
			foundSkip = true
			if res.Err != nil {
				t.Errorf("expected no error for success2, got %v", res.Err)
			}
			if !res.Skipped {
				t.Errorf("expected success2 to be skipped")
			}
		case "success1":
			foundSuccess = true
			if res.Err != nil {
				t.Errorf("expected no error for success1, got %v", res.Err)
			}
			if res.Skipped {
				t.Errorf("expected success1 not to be skipped")
			}
		}
	}

	if !foundFail || !foundSkip || !foundSuccess {
		t.Errorf("did not find all expected task outcomes: fail=%t, skip=%t, success=%t", foundFail, foundSkip, foundSuccess)
	}
}

func TestExecuteTasks_FailFast(t *testing.T) {
	tasks := []RepoTask{
		{Name: "fail_instant"},
		{Name: "slow1"},
		{Name: "slow2"},
	}

	action := func(ctx context.Context, task RepoTask) (error, string, bool) {
		if task.Name == "fail_instant" {
			// Small sleep to ensure others have started/are about to start
			time.Sleep(2 * time.Millisecond)
			return errors.New("instant error"), "fail_instant error", false
		}

		select {
		case <-ctx.Done():
			return ctx.Err(), "cancelled", false
		case <-time.After(100 * time.Millisecond):
			return nil, "", false
		}
	}

	results := ExecuteTasks(context.Background(), tasks, 3, true, 1*time.Second, action, nil)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Check that fail_instant failed, and slow tasks were cancelled
	var foundFail bool
	var foundCancelled bool

	for _, res := range results {
		if res.RepoName == "fail_instant" {
			foundFail = true
			if res.Err == nil || res.Err.Error() != "instant error" {
				t.Errorf("expected 'instant error', got %v", res.Err)
			}
		} else {
			// Because of fail-fast, these should be cancelled/interrupted
			if res.Err != nil && (errors.Is(res.Err, context.Canceled) || res.Skipped) {
				foundCancelled = true
			}
		}
	}

	if !foundFail {
		t.Error("fail_instant task result not found or did not fail")
	}
	if !foundCancelled {
		t.Error("expected at least one slow task to be cancelled/skipped due to fail-fast")
	}
}

func TestExecuteTasks_Timeout(t *testing.T) {
	tasks := []RepoTask{
		{Name: "timeout_task"},
	}

	action := func(ctx context.Context, task RepoTask) (error, string, bool) {
		select {
		case <-ctx.Done():
			return ctx.Err(), "timeout reached", false
		case <-time.After(100 * time.Millisecond):
			return nil, "", false
		}
	}

	// Set timeout to 10 milliseconds, which is less than 100ms
	results := ExecuteTasks(context.Background(), tasks, 1, false, 10*time.Millisecond, action, nil)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	res := results[0]
	if res.Err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(res.Err, context.DeadlineExceeded) && !errors.Is(res.Err, context.Canceled) {
		t.Errorf("expected deadline exceeded or canceled error, got %v", res.Err)
	}
}

func TestExecuteTasks_FailFast_Queued(t *testing.T) {
	tasks := []RepoTask{
		{Name: "fail_instant"},
		{Name: "queued1"},
	}

	action := func(ctx context.Context, task RepoTask) (error, string, bool) {
		if task.Name == "fail_instant" {
			return errors.New("instant error"), "fail_instant error", false
		}
		return nil, "", false
	}

	// Concurrency 1 means queued1 must wait until fail_instant finishes.
	// Since fail_instant fails and failFast is true, queued1 should be aborted before running its action.
	results := ExecuteTasks(context.Background(), tasks, 1, true, 1*time.Second, action, nil)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	var foundQueued bool
	for _, res := range results {
		if res.RepoName == "queued1" {
			foundQueued = true
			if res.Err == nil || !errors.Is(res.Err, context.Canceled) {
				t.Errorf("expected context.Canceled error for queued1, got %v", res.Err)
			}
			if !res.Skipped {
				t.Errorf("expected queued1 to be marked as skipped")
			}
		}
	}
	if !foundQueued {
		t.Error("queued1 task result not found")
	}
}

// TestExecuteTasks_EventEmission verifies that when an event channel is
// provided, ExecuteTasks dispatches the expected sequence of lifecycle
// events (started, then finished or skipped) for successful, failed,
// skipped, and fail-fast-cancelled task scenarios.
func TestExecuteTasks_EventEmission(t *testing.T) {
	tasks := []RepoTask{
		{Name: "success"},
		{Name: "fail"},
		{Name: "skip"},
	}

	action := func(ctx context.Context, task RepoTask) (error, string, bool) {
		switch task.Name {
		case "fail":
			return errors.New("boom"), "stderr text", false
		case "skip":
			return nil, "", true
		default:
			return nil, "", false
		}
	}

	eventChan := make(chan TaskEvent, len(tasks)*2)
	results := ExecuteTasks(context.Background(), tasks, 3, false, 1*time.Second, action, eventChan)
	close(eventChan)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	type seenEvents struct {
		started  bool
		finished bool
		skipped  bool
		err      error
		stderr   string
	}
	seen := make(map[string]*seenEvents)

	for event := range eventChan {
		s, ok := seen[event.RepoName]
		if !ok {
			s = &seenEvents{}
			seen[event.RepoName] = s
		}
		switch event.Type {
		case TaskStarted:
			s.started = true
		case TaskFinished:
			s.finished = true
			s.err = event.Err
			s.stderr = event.Stderr
		case TaskSkipped:
			s.skipped = true
			s.err = event.Err
		default:
			t.Errorf("unexpected event type %q for repo %q", event.Type, event.RepoName)
		}
	}

	successEvents, ok := seen["success"]
	if !ok {
		t.Fatal("expected events for 'success' task, found none")
	}
	if !successEvents.started || !successEvents.finished || successEvents.skipped {
		t.Errorf("expected success task to have started+finished (no skip), got %+v", successEvents)
	}
	if successEvents.err != nil {
		t.Errorf("expected no error for success task, got %v", successEvents.err)
	}

	failEvents, ok := seen["fail"]
	if !ok {
		t.Fatal("expected events for 'fail' task, found none")
	}
	if !failEvents.started || !failEvents.finished || failEvents.skipped {
		t.Errorf("expected fail task to have started+finished (no skip), got %+v", failEvents)
	}
	if failEvents.err == nil || failEvents.err.Error() != "boom" {
		t.Errorf("expected fail task error 'boom', got %v", failEvents.err)
	}
	if failEvents.stderr != "stderr text" {
		t.Errorf("expected fail task stderr 'stderr text', got %q", failEvents.stderr)
	}

	skipEvents, ok := seen["skip"]
	if !ok {
		t.Fatal("expected events for 'skip' task, found none")
	}
	if !skipEvents.started || skipEvents.finished || !skipEvents.skipped {
		t.Errorf("expected skip task to have started+skipped (no finished), got %+v", skipEvents)
	}
}

// TestExecuteTasks_EventEmission_FailFastCancelled verifies that tasks
// cancelled before running due to a sibling's fail-fast failure dispatch
// exactly one TaskSkipped event each, carrying the context cancellation
// error, without ever emitting a TaskStarted event for the cancelled task.
// Regression test for CR-01/D-05: the early-exit branch must write
// results[i] before emitting its event so the post-Wait() fill-in loop does
// not treat the same task as unhandled and emit a duplicate TaskSkipped
// event.
func TestExecuteTasks_EventEmission_FailFastCancelled(t *testing.T) {
	tasks := []RepoTask{
		{Name: "fail_instant"},
		{Name: "queued1"},
		{Name: "queued2"},
	}

	action := func(ctx context.Context, task RepoTask) (error, string, bool) {
		if task.Name == "fail_instant" {
			return errors.New("instant error"), "", false
		}
		return nil, "", false
	}

	eventChan := make(chan TaskEvent, len(tasks)*2)
	// Concurrency 1 forces queued1 and queued2 to wait behind fail_instant;
	// with fail-fast enabled, both should be cancelled before their action
	// runs.
	ExecuteTasks(context.Background(), tasks, 1, true, 1*time.Second, action, eventChan)
	close(eventChan)

	var queuedSkippedCount int
	var queuedStarted bool
	var queued2SkippedCount int
	var queued2Started bool
	for event := range eventChan {
		switch event.RepoName {
		case "queued1":
			switch event.Type {
			case TaskStarted:
				queuedStarted = true
			case TaskSkipped:
				queuedSkippedCount++
				if event.Err == nil || !errors.Is(event.Err, context.Canceled) {
					t.Errorf("expected context.Canceled on skipped event for queued1, got %v", event.Err)
				}
			}
		case "queued2":
			switch event.Type {
			case TaskStarted:
				queued2Started = true
			case TaskSkipped:
				queued2SkippedCount++
				if event.Err == nil || !errors.Is(event.Err, context.Canceled) {
					t.Errorf("expected context.Canceled on skipped event for queued2, got %v", event.Err)
				}
			}
		}
	}

	if queuedStarted {
		t.Error("expected queued1 NOT to emit TaskStarted (cancelled before running)")
	}
	if queuedSkippedCount != 1 {
		t.Errorf("expected exactly 1 TaskSkipped event for queued1, got %d", queuedSkippedCount)
	}
	if queued2Started {
		t.Error("expected queued2 NOT to emit TaskStarted (cancelled before running)")
	}
	if queued2SkippedCount != 1 {
		t.Errorf("expected exactly 1 TaskSkipped event for queued2, got %d", queued2SkippedCount)
	}
}
