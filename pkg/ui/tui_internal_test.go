package ui

import (
	"testing"

	"gitmera/pkg/runner"
)

// TestUpdate_SpinnerTickMsg_WhenQuitting verifies that a spinnerTickMsg received
// while m.quitting=true returns a nil cmd (no further ticking after quit).
func TestUpdate_SpinnerTickMsg_WhenQuitting(t *testing.T) {
	tasks := []runner.RepoTask{{Name: "api"}}
	ch := make(chan runner.TaskEvent, 1)
	m := NewTUIModel(tasks, "cloning", ch, nil)
	m.quitting = true

	_, cmd := m.Update(spinnerTickMsg{})
	if cmd != nil {
		t.Error("expected nil cmd when model is quitting")
	}
}

// TestUpdate_SpinnerTickMsg_WhenNotQuitting verifies that a spinnerTickMsg
// advances the spinner frame and schedules the next tick.
func TestUpdate_SpinnerTickMsg_WhenNotQuitting(t *testing.T) {
	tasks := []runner.RepoTask{{Name: "api"}}
	ch := make(chan runner.TaskEvent, 1)
	m := NewTUIModel(tasks, "cloning", ch, nil)

	updated, cmd := m.Update(spinnerTickMsg{})
	m2 := updated.(TUIModel)

	if cmd == nil {
		t.Error("expected non-nil cmd (tickSpinner) when not quitting")
	}
	if m2.spinnerFrame != 1 {
		t.Errorf("expected spinnerFrame=1 after one tick, got %d", m2.spinnerFrame)
	}
}

// TestUpdate_EventClosedMsg verifies that eventClosedMsg sets closed and quitting
// and returns a tea.Quit cmd.
func TestUpdate_EventClosedMsg(t *testing.T) {
	tasks := []runner.RepoTask{{Name: "api"}}
	ch := make(chan runner.TaskEvent, 1)
	m := NewTUIModel(tasks, "cloning", ch, nil)

	updated, cmd := m.Update(eventClosedMsg{})
	m2 := updated.(TUIModel)

	if !m2.closed {
		t.Error("expected closed=true after eventClosedMsg")
	}
	if !m2.quitting {
		t.Error("expected quitting=true after eventClosedMsg")
	}
	if cmd == nil {
		t.Error("expected non-nil cmd (tea.Quit) after eventClosedMsg")
	}
}

// TestUpdate_TaskEvent_WhenAlreadyClosed verifies that receiving a TaskEvent
// after the model is already closed returns a nil cmd (no further waiting).
func TestUpdate_TaskEvent_WhenAlreadyClosed(t *testing.T) {
	tasks := []runner.RepoTask{{Name: "api"}}
	ch := make(chan runner.TaskEvent, 1)
	m := NewTUIModel(tasks, "cloning", ch, nil)
	m.closed = true

	_, cmd := m.Update(runner.TaskEvent{RepoName: "api", Type: runner.TaskStarted})
	if cmd != nil {
		t.Error("expected nil cmd when model is already closed")
	}
}

// TestUpdate_UnhandledMessageType verifies that an unknown message type leaves
// the model unchanged and returns a nil cmd.
func TestUpdate_UnhandledMessageType(t *testing.T) {
	tasks := []runner.RepoTask{{Name: "api"}}
	ch := make(chan runner.TaskEvent, 1)
	m := NewTUIModel(tasks, "cloning", ch, nil)

	type unknownMsg struct{ data string }
	updated, cmd := m.Update(unknownMsg{"irrelevant"})
	m2 := updated.(TUIModel)

	if cmd != nil {
		t.Error("expected nil cmd for unhandled message type")
	}
	if m2.quitting {
		t.Error("expected quitting=false for unhandled message type")
	}
}

// TestWaitForEvent_ReceivesEvent verifies that the cmd returned by waitForEvent
// yields the event as a tea.Msg when the channel has data.
func TestWaitForEvent_ReceivesEvent(t *testing.T) {
	ch := make(chan runner.TaskEvent, 1)
	expected := runner.TaskEvent{RepoName: "api", Type: runner.TaskStarted}
	ch <- expected

	cmd := waitForEvent(ch)
	msg := cmd()

	event, ok := msg.(runner.TaskEvent)
	if !ok {
		t.Fatalf("expected runner.TaskEvent, got %T", msg)
	}
	if event.RepoName != expected.RepoName || event.Type != expected.Type {
		t.Errorf("expected event %+v, got %+v", expected, event)
	}
}

// TestWaitForEvent_ClosedChannel verifies that the cmd returned by waitForEvent
// yields an eventClosedMsg when the channel is closed.
func TestWaitForEvent_ClosedChannel(t *testing.T) {
	ch := make(chan runner.TaskEvent)
	close(ch)

	cmd := waitForEvent(ch)
	msg := cmd()

	if _, ok := msg.(eventClosedMsg); !ok {
		t.Fatalf("expected eventClosedMsg, got %T", msg)
	}
}

// TestTickSpinner_ReturnsNonNilCmd verifies that tickSpinner produces a
// non-nil tea.Cmd.
func TestTickSpinner_ReturnsNonNilCmd(t *testing.T) {
	cmd := tickSpinner()
	if cmd == nil {
		t.Error("expected non-nil cmd from tickSpinner")
	}
}

// TestTickSpinner_InnerCallbackExecuted invokes the command returned by
// tickSpinner directly (as the BubbleTea runtime would) to exercise the inner
// 100ms tick callback, covering the branch that returns spinnerTickMsg{}.
// This blocks for ~100ms while the timer fires.
func TestTickSpinner_InnerCallbackExecuted(t *testing.T) {
	cmd := tickSpinner()
	msg := cmd() // blocks ~100ms until the tick fires
	if _, ok := msg.(spinnerTickMsg); !ok {
		t.Errorf("expected spinnerTickMsg from tickSpinner inner callback, got %T", msg)
	}
}
