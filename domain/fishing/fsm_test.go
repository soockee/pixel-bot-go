package fishing

import (
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

// actionLog records the side effects the FSM requests, in order.
type actionLog struct {
	mu     sync.Mutex
	events []string
}

func (a *actionLog) add(s string) { a.mu.Lock(); a.events = append(a.events, s); a.mu.Unlock() }
func (a *actionLog) snapshot() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.events)
}

// testConfig is the default config with a short cooldown.
func testConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.CooldownSeconds = 1
	return cfg
}

func recordingActions(log *actionLog) ActionCallbacks {
	return ActionCallbacks{
		PressKey:   func(byte) { log.add("press") },
		MoveCursor: func(x, y int) { log.add("move") },
		ClickRight: func() { log.add("click") },
		ParseVK:    func(string) (byte, bool) { return 0, true },
	}
}

func newRecordingFSM(log *actionLog) *FishingFSM {
	return NewFSM(nil, testConfig(), recordingActions(log))
}

// fsmTest runs body on a fake clock, so timers fire instantly and in order.
func fsmTest(t *testing.T, body func(t *testing.T, m *FishingFSM, log *actionLog)) {
	t.Helper()
	synctest.Test(t, func(t *testing.T) {
		log := &actionLog{}
		m := newRecordingFSM(log)
		defer m.Close()
		body(t, m, log)
	})
}

func wantState(t *testing.T, m *FishingFSM, want FishingState) {
	t.Helper()
	synctest.Wait()
	if got := m.Current(); got != want {
		t.Fatalf("state = %v, want %v", got, want)
	}
}

func wantActions(t *testing.T, log *actionLog, want ...string) {
	t.Helper()
	synctest.Wait()
	if got := log.snapshot(); !slices.Equal(got, want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
}

// startMonitoring drives an FSM from halt to monitoring with a target at (10,10).
func startMonitoring(t *testing.T, m *FishingFSM) {
	t.Helper()
	m.EventAwaitFocus()
	m.EventFocusAcquired()
	m.EventTargetAcquiredAt(10, 10)
	wantState(t, m, StateMonitoring)
}

func TestFocusFlow(t *testing.T) {
	fsmTest(t, func(t *testing.T, m *FishingFSM, _ *actionLog) {
		m.EventAwaitFocus()
		wantState(t, m, StateWaitingFocus)
		m.EventFocusAcquired()
		wantState(t, m, StateSearching)
	})
}

func TestTargetLostRecasts(t *testing.T) {
	fsmTest(t, func(t *testing.T, m *FishingFSM, log *actionLog) {
		startMonitoring(t, m)
		m.EventTargetLost()
		wantState(t, m, StateSearching)
		wantActions(t, log, "move", "press")
	})
}

func TestForceCastFromMonitoring(t *testing.T) {
	fsmTest(t, func(t *testing.T, m *FishingFSM, log *actionLog) {
		startMonitoring(t, m)
		m.ForceCast()
		wantState(t, m, StateSearching)
		wantActions(t, log, "move", "press")
	})
}

func TestCooldownEndsWithACast(t *testing.T) {
	fsmTest(t, func(t *testing.T, m *FishingFSM, log *actionLog) {
		startMonitoring(t, m)
		m.EventFishBite()
		time.Sleep(reelClickDelay)
		wantState(t, m, StateCooldown)
		time.Sleep(time.Second)
		wantState(t, m, StateSearching)
		wantActions(t, log, "move", "move", "click", "press")
	})
}

func TestSearchTimeoutCasts(t *testing.T) {
	fsmTest(t, func(t *testing.T, m *FishingFSM, log *actionLog) {
		m.EventAwaitFocus()
		m.EventFocusAcquired()
		wantActions(t, log)
		time.Sleep(searchTimeout)
		wantActions(t, log, "press")
		wantState(t, m, StateSearching)
	})
}

func TestMonitoringTimeoutCasts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := testConfig()
		cfg.MaxCastDurationSeconds = 30
		log := &actionLog{}
		m := NewFSM(nil, cfg, recordingActions(log))
		defer m.Close()
		startMonitoring(t, m)

		time.Sleep(29 * time.Second)
		wantState(t, m, StateMonitoring)
		time.Sleep(time.Second)
		wantState(t, m, StateSearching)
		wantActions(t, log, "move", "press")
	})
}

func TestBiteOutsideMonitoringIsIgnored(t *testing.T) {
	fsmTest(t, func(t *testing.T, m *FishingFSM, _ *actionLog) {
		m.EventAwaitFocus()
		m.EventFishBite()
		wantState(t, m, StateWaitingFocus)
	})
}

// Listeners see every state, including the short casting state.
func TestListenersSeeEveryTransition(t *testing.T) {
	fsmTest(t, func(t *testing.T, m *FishingFSM, _ *actionLog) {
		var seen []FishingState
		m.AddListener(func(_, next FishingState) { seen = append(seen, next) })
		startMonitoring(t, m)
		m.EventTargetLost()
		synctest.Wait()
		want := []FishingState{StateWaitingFocus, StateSearching, StateMonitoring, StateCasting, StateSearching}
		if !slices.Equal(seen, want) {
			t.Errorf("seen = %v, want %v", seen, want)
		}
	})
}

func TestStatsCountCastsReelsAndFailures(t *testing.T) {
	fsmTest(t, func(t *testing.T, m *FishingFSM, _ *actionLog) {
		startMonitoring(t, m)
		m.EventTargetLost() // a cast without a reel
		wantState(t, m, StateSearching)
		if got, want := m.Stats(), (Stats{Casts: 1, FailedInRow: 1}); got != want {
			t.Fatalf("after a lost target: %+v, want %+v", got, want)
		}
		m.EventTargetAcquiredAt(10, 10)
		wantState(t, m, StateMonitoring)
		m.EventFishBite()
		time.Sleep(reelClickDelay + time.Second)
		wantState(t, m, StateSearching)
		if got, want := m.Stats(), (Stats{Casts: 2, Reels: 1}); got != want {
			t.Fatalf("after a reel and recast: %+v, want %+v", got, want)
		}
		m.EventHalt()
		m.EventAwaitFocus()
		wantState(t, m, StateWaitingFocus)
		if got := m.Stats(); got != (Stats{}) {
			t.Fatalf("a new run starts at %+v", got)
		}
	})
}
