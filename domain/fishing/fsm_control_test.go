package fishing

import (
	"testing"
	"testing/synctest"
	"time"
)

func TestStaleDeadlineIsIgnored(t *testing.T) {
	fsmTest(t, func(t *testing.T, m *FishingFSM, log *actionLog) {
		startMonitoring(t, m)
		m.send(evtDeadline{gen: 0})
		wantState(t, m, StateMonitoring)
		wantActions(t, log, "move")
	})
}

func TestStaleCastDoesNotLeaveHalt(t *testing.T) {
	fsmTest(t, func(t *testing.T, m *FishingFSM, log *actionLog) {
		m.ForceCast()
		wantState(t, m, StateHalt)
		wantActions(t, log)
	})
}

func TestStopCancelsPendingClick(t *testing.T) {
	fsmTest(t, func(t *testing.T, m *FishingFSM, log *actionLog) {
		startMonitoring(t, m)
		m.EventFishBite()
		time.Sleep(50 * time.Millisecond) // inside the delay between move and click
		m.EventHalt()
		time.Sleep(time.Minute)
		wantActions(t, log, "move", "move")
	})
}

func TestLateSearchResultKeepsTarget(t *testing.T) {
	fsmTest(t, func(t *testing.T, m *FishingFSM, _ *actionLog) {
		startMonitoring(t, m)
		m.EventTargetAcquiredAt(50, 50)
		synctest.Wait()
		if x, y, _ := m.TargetCoordinates(); x != 10 || y != 10 {
			t.Errorf("target = (%d,%d), want (10,10)", x, y)
		}
	})
}

func TestEventsAfterCloseAreSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic: %v", r)
		}
	}()
	m := NewFSM(nil, testConfig(), recordingActions(&actionLog{}))
	m.Close()
	m.Close()
	m.EventFocusAcquired()
}

func TestReelingMovesThenClicksThenCoolsDown(t *testing.T) {
	fsmTest(t, func(t *testing.T, m *FishingFSM, log *actionLog) {
		startMonitoring(t, m)
		m.EventFishBite()
		wantState(t, m, StateReeling)
		time.Sleep(reelClickDelay)
		wantState(t, m, StateCooldown)
		wantActions(t, log, "move", "move", "click")
	})
}

func TestFocusLostCancelsPendingClick(t *testing.T) {
	fsmTest(t, func(t *testing.T, m *FishingFSM, log *actionLog) {
		startMonitoring(t, m)
		m.EventFishBite()
		synctest.Wait()
		m.EventFocusLost()
		time.Sleep(time.Minute)
		wantState(t, m, StateWaitingFocus)
		wantActions(t, log, "move", "move")
	})
}

func TestFocusReturnResumesSearching(t *testing.T) {
	fsmTest(t, func(t *testing.T, m *FishingFSM, _ *actionLog) {
		startMonitoring(t, m)
		m.EventFocusLost()
		wantState(t, m, StateWaitingFocus)
		m.EventFocusAcquired()
		wantState(t, m, StateSearching)
	})
}

func TestFocusLostInHaltDoesNothing(t *testing.T) {
	fsmTest(t, func(t *testing.T, m *FishingFSM, _ *actionLog) {
		m.EventFocusLost()
		wantState(t, m, StateHalt)
	})
}

func TestFailedCastsHaltAfterLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := testConfig()
		cfg.MaxFailedCasts = 2
		m := NewFSM(nil, cfg, recordingActions(&actionLog{}))
		defer m.Close()
		startMonitoring(t, m)

		m.EventTargetLost()
		wantState(t, m, StateSearching)
		m.EventTargetAcquiredAt(10, 10)
		m.EventTargetLost()

		wantState(t, m, StateHalt)
	})
}

func TestReelResetsFailedCasts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := testConfig()
		cfg.MaxFailedCasts = 2
		m := NewFSM(nil, cfg, recordingActions(&actionLog{}))
		defer m.Close()
		startMonitoring(t, m)

		m.EventTargetLost() // first failure
		m.EventTargetAcquiredAt(10, 10)
		m.EventFishBite()
		time.Sleep(reelClickDelay + time.Duration(cfg.CooldownSeconds)*time.Second)
		synctest.Wait()
		m.EventTargetAcquiredAt(10, 10)
		m.EventTargetLost() // would be the second failure in a row without the reel

		wantState(t, m, StateSearching)
	})
}

func TestSessionLimitHaltsAtTheNextCast(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := testConfig()
		cfg.MaxSessionMinutes = 1
		cfg.MaxFailedCasts = 1000
		log := &actionLog{}
		m := NewFSM(nil, cfg, recordingActions(log))
		defer m.Close()
		m.EventAwaitFocus()
		m.EventFocusAcquired()

		time.Sleep(time.Minute + searchTimeout)
		wantState(t, m, StateHalt)

		presses := len(log.snapshot())
		time.Sleep(time.Minute)
		synctest.Wait()
		if got := len(log.snapshot()); got != presses {
			t.Errorf("presses went from %d to %d after the limit", presses, got)
		}
	})
}

func TestStartUsesTheCurrentConfig(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := testConfig()
		m := NewFSM(nil, cfg, recordingActions(&actionLog{}))
		defer m.Close()
		cfg.CooldownSeconds = 30 // changed in the panel before Start

		startMonitoring(t, m)
		cfg.CooldownSeconds = 1 // a later change must not reach this run
		m.EventFishBite()
		time.Sleep(reelClickDelay + 29*time.Second)

		wantState(t, m, StateCooldown)
	})
}

func TestUnknownCastKeyHaltsWithoutPressing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := &actionLog{}
		actions := recordingActions(log)
		actions.ParseVK = func(string) (byte, bool) { return 0, false }
		m := NewFSM(nil, testConfig(), actions)
		defer m.Close()
		startMonitoring(t, m)

		m.EventTargetLost()

		wantState(t, m, StateHalt)
		wantActions(t, log, "move")
	})
}

func TestLoopPanicHaltsAndKeepsSendersFree(t *testing.T) {
	actions := recordingActions(&actionLog{})
	actions.ClickRight = func() bool { panic("input failed") }
	m := NewFSM(nil, testConfig(), actions)
	halted := make(chan struct{}, 1)
	m.AddListener(func(_, next FishingState) {
		if next == StateHalt {
			halted <- struct{}{}
		}
	})
	m.EventAwaitFocus()
	m.EventFocusAcquired()
	m.EventTargetAcquiredAt(10, 10)
	m.EventFishBite()

	select {
	case <-halted:
	case <-time.After(2 * time.Second):
		t.Fatal("listeners were not told about the halt")
	}
	for range 200 { // more than the event buffer holds
		m.EventHalt()
	}
}
