package presenter

import (
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/domain/fishing"
)

type mockFocusFSM struct {
	state fishing.FishingState
	fired int
}

func (m *mockFocusFSM) Current() fishing.FishingState { return m.state }
func (m *mockFocusFSM) EventFocusAcquired()           { m.fired++; m.state = fishing.StateSearching }

// Test that watcher fires only when foreground title changes and matches selection while waiting.
func TestFocusWatcher_FiresOnMatchChange(t *testing.T) {
	fsm := &mockFocusFSM{state: fishing.StateWaitingFocus}
	titles := []string{"Other", "GameWindow"}
	idx := 0
	fg := func() (string, error) { return titles[idx], nil }
	sel := func() string { return "gamewindow" }
	w := NewFocusWatcher(fsm, nil, fg, sel)

	// Start watcher via state transition
	w.OnState(fishing.StateSearching, fishing.StateWaitingFocus)
	// Foreground Other (not match)
	time.Sleep(300 * time.Millisecond)
	if fsm.fired != 0 {
		t.Fatalf("expected no fire, got %d", fsm.fired)
	}
	// Change to matching title
	idx = 1
	time.Sleep(300 * time.Millisecond)
	if fsm.fired != 1 {
		t.Fatalf("expected fire on match change, got %d", fsm.fired)
	}
	// Ensure it stopped
	time.Sleep(300 * time.Millisecond)
	if fsm.fired != 1 {
		t.Fatalf("unexpected repeat fire")
	}
}

// Test that leaving focus state resets and allows firing again.
func TestFocusWatcher_ResetOnStateExit(t *testing.T) {
	fsm := &mockFocusFSM{state: fishing.StateWaitingFocus}
	title := "WinA"
	fg := func() (string, error) { return title, nil }
	sel := func() string { return "wina" }
	w := NewFocusWatcher(fsm, nil, fg, sel)
	w.OnState(fishing.StateSearching, fishing.StateWaitingFocus)
	time.Sleep(300 * time.Millisecond)
	if fsm.fired != 1 {
		t.Fatalf("expected first fire, got %d", fsm.fired)
	}
	// Leave waiting state (should stop watcher)
	fsm.state = fishing.StateSearching
	w.OnState(fishing.StateWaitingFocus, fishing.StateSearching)
	// Re-enter waiting with different title then matching again
	fsm.state = fishing.StateWaitingFocus
	title = "Other"
	w.OnState(fishing.StateSearching, fishing.StateWaitingFocus)
	time.Sleep(300 * time.Millisecond) // no fire
	if fsm.fired != 1 {
		t.Fatalf("unexpected fire after re-enter with non-match")
	}
	title = "WinA"
	time.Sleep(300 * time.Millisecond)
	if fsm.fired != 2 {
		t.Fatalf("expected second fire after reset, got %d", fsm.fired)
	}
}
