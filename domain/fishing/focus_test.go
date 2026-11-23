package fishing

import (
	"errors"
	"image"
	"testing"
	"testing/synctest"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

// Input the Input dropped because the game lost focus pauses the run, which
// goes on once the game is back in front. Any other failure stops it.
func TestFocusRaceAtInputPauses(t *testing.T) {
	tests := []struct {
		name   string
		refuse func(*testInput, error)
		reach  func(*testing.T, *FishingFSM)
	}{
		{"cast", func(in *testInput, err error) {
			in.tap = func(byte, time.Duration) error { return err }
		}, func(t *testing.T, m *FishingFSM) {
			m.EventAwaitFocus()
			m.EventFocusAcquired()
			wantState(t, m, StateSearching)
			m.ForceCast()
		}},
		{"pointer", func(in *testInput, err error) {
			in.move = func(image.Point) error { return err }
		}, func(t *testing.T, m *FishingFSM) {
			m.EventAwaitFocus()
			m.EventFocusAcquired()
			m.EventTargetAcquiredAt(10, 10) // the move onto the bobber is refused
		}},
		{"reel", func(in *testInput, err error) {
			in.click = func() error { return err }
		}, func(t *testing.T, m *FishingFSM) {
			startMonitoring(t, m)
			m.EventFishBite()
			time.Sleep(reelClickDelay)
		}},
	}
	for _, tt := range tests {
		for _, focus := range []bool{true, false} {
			name := tt.name + "/board failed"
			err := errors.New("ESP32 tap acknowledgment: i/o timeout")
			if focus {
				name, err = tt.name+"/focus lost", ErrNotInFront
			}
			t.Run(name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					actions := recordingActions(&actionLog{})
					tt.refuse(inputOf(actions), err)
					m := NewFSM(nil, testConfig(), actions)
					defer m.Close()
					tt.reach(t, m)
					if !focus {
						wantState(t, m, StateHalt)
						if m.HaltReason() == "" {
							t.Error("stopped without a reason")
						}
						return
					}
					wantState(t, m, StateWaitingFocus)
					if got := m.HaltReason(); got != "" {
						t.Errorf("a pause recorded a halt reason: %q", got)
					}
					m.EventFocusAcquired()
					wantState(t, m, StateSearching)
				})
			})
		}
	}
}

// A reposition key the Input dropped for focus pauses instead of stopping.
func TestFocusRaceAtRepositionPauses(t *testing.T) {
	f := &FishingFSM{
		cfg:    config.Config{Reposition: true},
		logger: testLogger,
		state:  StateCooldown,
		actions: ActionCallbacks{
			InFront:  func() bool { return true },
			Pointer:  &testPointer{},
			Observer: NopObserver{},
			PlanReel: func(string) ReelTiming { return ReelTiming{} },
			Roll:     func(int) int { return 0 },
			Input:    &testInput{tap: func(byte, time.Duration) error { return ErrNotInFront }},
		},
	}
	f.startReposition()
	if f.state != StateWaitingFocus {
		t.Fatalf("state = %v, want paused", f.state)
	}
	if got := f.HaltReason(); got != "" {
		t.Errorf("a pause recorded a halt reason: %q", got)
	}
}
