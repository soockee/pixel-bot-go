package fishing

import (
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestHaltOnItsOwnSaysWhy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := testConfig()
		cfg.MaxFailedCasts = 2
		m := NewFSM(nil, cfg, recordingActions(&actionLog{}))
		defer m.Close()
		startMonitoring(t, m)
		m.EventTargetLost()
		m.EventTargetAcquiredAt(10, 10)
		m.EventTargetLost()
		wantState(t, m, StateHalt)
		if got := m.HaltReason(); got != "2 casts in a row got no bite" {
			t.Fatalf("HaltReason = %q", got)
		}
	})
}

func TestSessionLimitSaysWhy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := testConfig()
		cfg.MaxFailedCasts, cfg.MaxSessionMinutes = 1000, 1
		m := NewFSM(nil, cfg, recordingActions(&actionLog{}))
		defer m.Close()
		startMonitoring(t, m)
		time.Sleep(time.Minute + searchTimeout)
		synctest.Wait()
		wantState(t, m, StateHalt)
		if got := m.HaltReason(); !strings.Contains(got, "1 minute session limit") {
			t.Fatalf("HaltReason = %q", got)
		}
	})
}

func TestHaltBecauseKeepsTheReason(t *testing.T) {
	fsmTest(t, func(t *testing.T, m *FishingFSM, _ *actionLog) {
		startMonitoring(t, m)
		m.EventHaltBecause("the mouse was moved")
		wantState(t, m, StateHalt)
		if got := m.HaltReason(); got != "the mouse was moved" {
			t.Fatalf("HaltReason = %q", got)
		}
	})
}

func TestStopTheUserAskedForHasNoReason(t *testing.T) {
	fsmTest(t, func(t *testing.T, m *FishingFSM, _ *actionLog) {
		startMonitoring(t, m)
		m.EventHalt()
		wantState(t, m, StateHalt)
		if got := m.HaltReason(); got != "" {
			t.Fatalf("HaltReason = %q after a stop the person asked for", got)
		}
	})
}

func TestNewRunForgetsTheLastReason(t *testing.T) {
	fsmTest(t, func(t *testing.T, m *FishingFSM, _ *actionLog) {
		startMonitoring(t, m)
		m.EventHaltBecause("the mouse was moved")
		wantState(t, m, StateHalt)

		m.EventAwaitFocus()
		wantState(t, m, StateWaitingFocus)
		if got := m.HaltReason(); got != "" {
			t.Fatalf("HaltReason = %q in the next run", got)
		}
	})
}

func TestUnknownCastKeySaysWhy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		actions := recordingActions(&actionLog{})
		actions.ParseVK = func(string) (byte, bool) { return 0, false }
		m := NewFSM(nil, testConfig(), actions)
		defer m.Close()
		m.EventAwaitFocus()
		m.EventFocusAcquired()
		wantState(t, m, StateSearching)
		m.ForceCast()
		wantState(t, m, StateHalt)
		if got := m.HaltReason(); !strings.Contains(got, "is not a Cast key") {
			t.Fatalf("HaltReason = %q", got)
		}
	})
}
