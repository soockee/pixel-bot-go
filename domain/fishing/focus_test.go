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
	causes := []struct {
		name string
		err  error
	}{
		{"focus lost", ErrNotInFront},
		{"mouse moved", ErrMouseMoved},
		{"board failed", errors.New("ESP32 tap acknowledgment: i/o timeout")},
	}
	for _, tt := range tests {
		for _, cause := range causes {
			t.Run(tt.name+"/"+cause.name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					deps := recordingDeps(&actionLog{})
					tt.refuse(inputOf(deps), cause.err)
					m := NewFSM(nil, testConfig(), deps)
					defer m.Close()
					tt.reach(t, m)
					switch cause.err {
					case ErrNotInFront, ErrMouseMoved:
					default:
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
					if byMouse := cause.err == ErrMouseMoved; m.PausedByMouse() != byMouse {
						t.Fatalf("PausedByMouse = %v, want %v", !byMouse, byMouse)
					}
					m.EventFocusAcquired()
					if cause.err == ErrMouseMoved {
						wantState(t, m, StateWaitingFocus) // focus alone does not end it
						m.EventMouseSettled()
					}
					wantState(t, m, StateSearching)
					if m.PausedByMouse() {
						t.Error("still paused by the mouse after it rested")
					}
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
		deps: Deps{
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

// The Board test of 2026-11-23: the person moved the mouse just as the bot
// planned its path to a new bobber. The plan's refusal pauses the run.
func TestMovedMouseAtAimPauses(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		deps := recordingDeps(&actionLog{})
		pointerOf(deps).plan = func(image.Point, bool) ([]CursorStep, error) { return nil, ErrMouseMoved }
		m := NewFSM(nil, testConfig(), deps)
		defer m.Close()
		m.EventAwaitFocus()
		m.EventFocusAcquired()
		m.EventTargetAcquiredAt(10, 10)
		wantState(t, m, StateWaitingFocus)
		if !m.PausedByMouse() || m.HaltReason() != "" {
			t.Fatalf("PausedByMouse = %v, halt reason %q", m.PausedByMouse(), m.HaltReason())
		}
	})
}
