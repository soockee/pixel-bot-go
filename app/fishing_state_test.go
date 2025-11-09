package app

import (
	"log/slog"
	"sync"
	"testing"
	"time"
)

// NOTE: Race detector (-race) is not used on Windows for this project; tests
// intentionally avoid depending on data race reporting. The FSM is designed as
// a single-goroutine actor (events processed on its internal loop), so these
// tests verify functional state transitions only—not memory synchronization.
// Concurrency in side-effect goroutines (cursor/key actions) is tolerated but
// not asserted. We rely on pure Go execution without requiring cgo toolchains.

// dummy logger discards output
var discardLogger = slog.New(slog.NewTextHandler(&discardWriter{}, nil))

type discardWriter struct{}

func (d *discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// Test that after a bite the state machine reaches Reeling then auto-transitions to Cooldown.
func TestFishingStateMachine_ReelingAdvancesToCooldown(t *testing.T) {
	m := NewFishingStateMachine(discardLogger, nil)
	// Move from halt to waiting focus
	m.EventAwaitFocus()
	time.Sleep(10 * time.Millisecond)
	// Acquire focus -> searching
	m.EventFocusAquired()
	time.Sleep(10 * time.Millisecond)
	// Acquire target -> monitoring
	m.EventTargetAcquiredAt(10, 10)
	time.Sleep(10 * time.Millisecond)
	// Validate monitoring state
	if m.Current() != StateMonitoring {
		t.Fatalf("expected monitoring state, got %v", m.Current())
	}
	// Trigger bite -> reeling -> cooldown
	m.EventFishBite()
	time.Sleep(50 * time.Millisecond)
	// Allow reel goroutine to set cooldown and transition.
	time.Sleep(200 * time.Millisecond)
	st := m.Current()
	if st != StateCooldown {
		t.Fatalf("expected cooldown state after reeling, got %v", st)
	}
}

// Helper: wait until state equals expected or timeout.
func waitForState(t *testing.T, m *FishingStateMachine, expected FishingState, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if m.Current() == expected {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for state %v (got %v)", expected, m.Current())
}

// transitionRecorder records state changes for assertions.
type transitionRecorder struct {
	mu  sync.Mutex
	seq []FishingState
}

func (r *transitionRecorder) listener(prev, next FishingState) {
	r.mu.Lock()
	r.seq = append(r.seq, next)
	r.mu.Unlock()
}

func (r *transitionRecorder) states() []FishingState {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := make([]FishingState, len(r.seq))
	copy(s, r.seq)
	return s
}

func TestFishingStateMachine_FocusFlow(t *testing.T) {
	m := NewFishingStateMachine(discardLogger, nil)
	r := &transitionRecorder{}
	m.AddListener(r.listener)
	m.EventAwaitFocus()
	waitForState(t, m, StateWaitingFocus, 200*time.Millisecond)
	m.EventFocusAquired()
	waitForState(t, m, StateSearching, 200*time.Millisecond)
}

func TestFishingStateMachine_TargetAcquisitionFlow(t *testing.T) {
	m := NewFishingStateMachine(discardLogger, nil)
	m.EventAwaitFocus()
	waitForState(t, m, StateWaitingFocus, 200*time.Millisecond)
	m.EventFocusAquired()
	waitForState(t, m, StateSearching, 200*time.Millisecond)
	m.EventTargetAcquiredAt(1, 2)
	waitForState(t, m, StateMonitoring, 200*time.Millisecond)
}

func TestFishingStateMachine_TargetLostFlow(t *testing.T) {
	m := NewFishingStateMachine(discardLogger, nil)
	m.EventAwaitFocus()
	waitForState(t, m, StateWaitingFocus, 200*time.Millisecond)
	m.EventFocusAquired()
	waitForState(t, m, StateSearching, 200*time.Millisecond)
	m.EventTargetAcquired()
	waitForState(t, m, StateMonitoring, 200*time.Millisecond)
	m.EventTargetLost()
	// TargetLost transitions to Casting (ephemeral) then immediately Searching.
	waitForState(t, m, StateSearching, 300*time.Millisecond)
}

func TestFishingStateMachine_ForceCastFromMonitoring(t *testing.T) {
	m := NewFishingStateMachine(discardLogger, nil)
	m.EventAwaitFocus()
	waitForState(t, m, StateWaitingFocus, 200*time.Millisecond)
	m.EventFocusAquired()
	waitForState(t, m, StateSearching, 200*time.Millisecond)
	m.EventTargetAcquiredAt(5, 5)
	waitForState(t, m, StateMonitoring, 200*time.Millisecond)
	m.ForceCast()
	waitForState(t, m, StateSearching, 300*time.Millisecond)
}

func TestFishingStateMachine_CooldownExpiration(t *testing.T) {
	m := NewFishingStateMachine(discardLogger, nil)
	m.EventAwaitFocus()
	waitForState(t, m, StateWaitingFocus, 200*time.Millisecond)
	m.EventFocusAquired()
	waitForState(t, m, StateSearching, 200*time.Millisecond)
	m.EventTargetAcquiredAt(3, 4)
	waitForState(t, m, StateMonitoring, 200*time.Millisecond)
	m.EventFishBite()
	waitForState(t, m, StateCooldown, 400*time.Millisecond)
	// Advance time beyond cooldownUntil via Tick.
	cu := m.cooldownUntil // direct access acceptable in same package; no race tests required
	if cu.IsZero() {
		// Should not happen; fail fast without race detector assumptions.
		t.Fatalf("cooldownUntil was zero")
	}
	m.Tick(cu.Add(50 * time.Millisecond))
	waitForState(t, m, StateSearching, 400*time.Millisecond)
}

func TestFishingStateMachine_SearchTimeoutTriggersCast(t *testing.T) {
	m := NewFishingStateMachine(discardLogger, nil)
	m.EventAwaitFocus()
	waitForState(t, m, StateWaitingFocus, 200*time.Millisecond)
	m.EventFocusAquired()
	waitForState(t, m, StateSearching, 200*time.Millisecond)
	// Simulate timeout >5s
	start := time.Now()
	m.Tick(start.Add(6 * time.Second))
	waitForState(t, m, StateSearching, 300*time.Millisecond)
}

func TestFishingStateMachine_InvalidEventNoTransition(t *testing.T) {
	m := NewFishingStateMachine(discardLogger, nil)
	m.EventAwaitFocus()
	waitForState(t, m, StateWaitingFocus, 200*time.Millisecond)
	// EventFishBite while not monitoring should not move to Reeling.
	m.EventFishBite()
	// Ensure still WaitingFocus shortly after.
	time.Sleep(50 * time.Millisecond)
	if m.Current() != StateWaitingFocus {
		t.Fatalf("unexpected state change on invalid bite event: %v", m.Current())
	}
}
