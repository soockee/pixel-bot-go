package fishing

import (
	"log/slog"
	"sync"
	"testing"
	"time"
)

// Functional transition tests.

var discardLogger = slog.New(slog.NewTextHandler(&discardWriter{}, nil))

type discardWriter struct{}

func (d *discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// newTestFSM returns an FSM with no-op action callbacks for testing.
func newTestFSM() *FishingFSM {
	return NewFSM(discardLogger, nil, ActionCallbacks{
		PressKey:   func(byte) {},
		MoveCursor: func(int, int) {},
		ClickRight: func() {},
		ParseVK:    func(string) byte { return 0 },
	}, nil)
}

func TestFishingFSM_ReelingAdvancesToCooldown(t *testing.T) {
	m := newTestFSM()
	m.EventAwaitFocus()
	time.Sleep(10 * time.Millisecond)
	m.EventFocusAcquired()
	time.Sleep(10 * time.Millisecond)
	m.EventTargetAcquiredAt(10, 10)
	time.Sleep(10 * time.Millisecond)
	if m.Current() != StateMonitoring {
		t.Fatalf("expected monitoring state, got %v", m.Current())
	}
	m.EventFishBite()
	time.Sleep(50 * time.Millisecond)
	time.Sleep(200 * time.Millisecond)
	if st := m.Current(); st != StateCooldown {
		t.Fatalf("expected cooldown state after reeling, got %v", st)
	}
}

// waitForState waits up to timeout for the FSM to reach expected state.
func waitForState(t *testing.T, m *FishingFSM, expected FishingState, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if m.Current() == expected {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for state %v (got %v)", expected, m.Current())
}

type transitionRecorder struct {
	mu  sync.Mutex
	seq []FishingState
}

// listener records transitions.
func (r *transitionRecorder) listener(prev, next FishingState) {
	r.mu.Lock()
	r.seq = append(r.seq, next)
	r.mu.Unlock()
}

func TestFishingFSM_FocusFlow(t *testing.T) {
	m := newTestFSM()
	r := &transitionRecorder{}
	m.AddListener(r.listener)
	m.EventAwaitFocus()
	waitForState(t, m, StateWaitingFocus, 200*time.Millisecond)
	m.EventFocusAcquired()
	waitForState(t, m, StateSearching, 200*time.Millisecond)
}

func TestFishingFSM_TargetAcquisitionFlow(t *testing.T) {
	m := newTestFSM()
	m.EventAwaitFocus()
	waitForState(t, m, StateWaitingFocus, 200*time.Millisecond)
	m.EventFocusAcquired()
	waitForState(t, m, StateSearching, 200*time.Millisecond)
	m.EventTargetAcquiredAt(1, 2)
	waitForState(t, m, StateMonitoring, 200*time.Millisecond)
}

func TestFishingFSM_TargetLostFlow(t *testing.T) {
	m := newTestFSM()
	m.EventAwaitFocus()
	waitForState(t, m, StateWaitingFocus, 200*time.Millisecond)
	m.EventFocusAcquired()
	waitForState(t, m, StateSearching, 200*time.Millisecond)
	m.EventTargetAcquired()
	waitForState(t, m, StateMonitoring, 200*time.Millisecond)
	m.EventTargetLost()
	waitForState(t, m, StateSearching, 300*time.Millisecond)
}

func TestFishingFSM_ForceCastFromMonitoring(t *testing.T) {
	m := newTestFSM()
	m.EventAwaitFocus()
	waitForState(t, m, StateWaitingFocus, 200*time.Millisecond)
	m.EventFocusAcquired()
	waitForState(t, m, StateSearching, 200*time.Millisecond)
	m.EventTargetAcquiredAt(5, 5)
	waitForState(t, m, StateMonitoring, 200*time.Millisecond)
	m.ForceCast()
	waitForState(t, m, StateSearching, 300*time.Millisecond)
}

func TestFishingFSM_CooldownExpiration(t *testing.T) {
	m := newTestFSM()
	m.EventAwaitFocus()
	waitForState(t, m, StateWaitingFocus, 200*time.Millisecond)
	m.EventFocusAcquired()
	waitForState(t, m, StateSearching, 200*time.Millisecond)
	m.EventTargetAcquiredAt(3, 4)
	waitForState(t, m, StateMonitoring, 200*time.Millisecond)
	m.EventFishBite()
	waitForState(t, m, StateCooldown, 400*time.Millisecond)
	// Expect automatic cast after cooldown.
	waitForState(t, m, StateSearching, 3*time.Second)
}

func TestFishingFSM_SearchTimeoutTriggersCast(t *testing.T) {
	m := newTestFSM()
	r := &transitionRecorder{}
	m.AddListener(r.listener)
	m.EventAwaitFocus()
	waitForState(t, m, StateWaitingFocus, 200*time.Millisecond)
	m.EventFocusAcquired()
	waitForState(t, m, StateSearching, 200*time.Millisecond)
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		for _, s := range r.seq {
			if s == StateCasting {
				r.mu.Unlock()
				return
			}
		}
		r.mu.Unlock()
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("expected casting transition within search timeout; got sequence %v", r.seq)
}

func TestFishingFSM_InvalidEventNoTransition(t *testing.T) {
	m := newTestFSM()
	m.EventAwaitFocus()
	waitForState(t, m, StateWaitingFocus, 200*time.Millisecond)
	m.EventFishBite()
	time.Sleep(50 * time.Millisecond)
	if m.Current() != StateWaitingFocus {
		t.Fatalf("unexpected change on invalid bite event: %v", m.Current())
	}
}
