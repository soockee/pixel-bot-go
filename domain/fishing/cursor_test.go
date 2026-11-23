package fishing

import (
	"errors"
	"image"
	"os"
	"testing"
	"testing/synctest"
	"time"

	"github.com/soocke/pixel-bot-go/internal/cursormotion"
	"github.com/soocke/pixel-bot-go/internal/fakegame"
)

func TestReelWaitsForCursorAndStopCancelsRemainingSteps(t *testing.T) {
	for _, stop := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			log := &actionLog{}
			deps := recordingDeps(log)
			deps.Pointer = &testPointer{
				plan: func(target image.Point, settle bool) ([]CursorStep, bool) {
					if settle {
						return []CursorStep{{After: time.Second, Point: target}}, true
					}
					return []CursorStep{
						{After: 200 * time.Millisecond, Point: target.Sub(image.Pt(5, 0))},
						{After: 600 * time.Millisecond, Point: target},
					}, true
				},
				reset: func() {},
			}
			inputOf(deps).move = func(p image.Point) error {
				if p == image.Pt(10, 10) {
					log.add("target")
				} else {
					log.add("waypoint")
				}
				return nil
			}
			fsm := NewFSM(nil, testConfig(), deps)
			defer fsm.Close()
			startMonitoring(t, fsm)
			fsm.EventFishBite()
			synctest.Wait()
			time.Sleep(500 * time.Millisecond)
			wantActions(t, log, "waypoint")
			if stop {
				fsm.EventHalt()
				time.Sleep(time.Second)
				wantActions(t, log, "waypoint")
				return
			}
			time.Sleep(100 * time.Millisecond)
			wantActions(t, log, "waypoint", "target", "click")
		})
	}
}

// The person may use the mouse while the game is not in front, so the pointer
// the bot left behind before a focus loss must not count as a takeover after it.
func TestFocusLossForgetsThePointer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		resets := 0
		deps := recordingDeps(&actionLog{})
		deps.Pointer = &testPointer{
			plan: func(target image.Point, _ bool) ([]CursorStep, bool) {
				return []CursorStep{{After: 100 * time.Millisecond, Point: target}}, true
			},
			reset: func() { resets++ },
		}
		fsm := NewFSM(nil, testConfig(), deps)
		defer fsm.Close()
		startMonitoring(t, fsm)
		before := resets
		fsm.EventFocusLost()
		wantState(t, fsm, StateWaitingFocus)
		if resets != before+1 {
			t.Fatalf("focus loss reset the pointer guard %d times, want once", resets-before)
		}
	})
}

func TestRefusedCursorStepHaltsBeforeReel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := &actionLog{}
		deps := recordingDeps(log)
		deps.Pointer = &testPointer{
			plan: func(target image.Point, _ bool) ([]CursorStep, bool) {
				return []CursorStep{{After: 100 * time.Millisecond, Point: target}}, true
			},
			reset: func() {},
		}
		inputOf(deps).move = func(image.Point) error { return errors.New("blocked") }
		fsm := NewFSM(nil, testConfig(), deps)
		defer fsm.Close()
		startMonitoring(t, fsm)
		fsm.EventFishBite()
		time.Sleep(reelClickDelay + time.Second)
		wantState(t, fsm, StateHalt)
		wantActions(t, log)
	})
}

func TestMissedPointerGivesUpTheCast(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := &actionLog{}
		deps := recordingDeps(log)
		deps.Pointer = &testPointer{
			plan: func(target image.Point, _ bool) ([]CursorStep, bool) {
				return []CursorStep{{After: 100 * time.Millisecond, Point: target}}, true
			},
			reset: func() {},
		}
		inputOf(deps).move = func(image.Point) error { return ErrPointerMissed }
		cfg := testConfig()
		cfg.MaxFailedCasts = 2
		fsm := NewFSM(nil, cfg, deps)
		defer fsm.Close()
		startMonitoring(t, fsm)
		fsm.EventFishBite()
		time.Sleep(reelClickDelay + time.Second)
		wantState(t, fsm, StateSearching) // one missed aim recast instead of halting
		fsm.EventTargetAcquiredAt(10, 10)
		fsm.EventFishBite()
		time.Sleep(reelClickDelay + time.Second)
		wantState(t, fsm, StateHalt) // the second miss in a row reached MaxFailedCasts
		wantActions(t, log, "press")
	})
}

func TestPairedCursorReplay(t *testing.T) {
	model := cursormotion.Model{Samples: []cursormotion.Sample{{
		Distance: 170,
		Points: []cursormotion.Point{
			{}, {US: 220_000, Along: .7, Across: .12}, {US: 370_000, Along: .98},
		},
		Settle: &cursormotion.Settle{PauseUS: 300_000, DurationUS: 80_000},
	}}}
	if path := os.Getenv("VRL_CURSOR_REPLAY_MODEL"); path != "" {
		var err error
		model, err = cursormotion.Load(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, profile := range []string{"direct", "fitted"} {
		t.Run(profile, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				bounds := image.Rect(-1000, 200, -520, 520)
				position := image.Pt(-960, 230)
				deps := recordingDeps(&actionLog{})
				var fsm *FishingFSM
				var presses, cues []time.Time
				var moves int
				inputOf(deps).click = func() error {
					x, y, ok := fsm.TargetCoordinates()
					if !ok || position != image.Pt(x, y) {
						t.Errorf("reel attempted at %v, target is (%d,%d)", position, x, y)
					}
					presses = append(presses, time.Now())
					return nil
				}
				inputOf(deps).move = func(p image.Point) error {
					position = p
					moves++
					if profile == "fitted" && !p.In(bounds) {
						return errors.New("outside")
					}
					return nil
				}
				if profile == "fitted" {
					planner := model.Planner(42)
					deps.Pointer = &testPointer{
						plan: func(target image.Point, settle bool) ([]CursorStep, bool) {
							path := planner(position, target, bounds, settle)
							steps := make([]CursorStep, len(path))
							for i, s := range path {
								steps[i] = CursorStep{After: s.After, Point: s.Point}
							}
							return steps, len(steps) > 0
						},
						reset: func() {},
					}
				}
				engine, runFSM, frames, _ := newSceneEngine(t, deps)
				fsm = runFSM
				engine.cfg.CooldownSeconds = 3
				defer fsm.Close()
				frames.set(fakegame.PhaseEmpty)
				startEngine(t, engine)
				defer engine.Stop()
				fsm.EventAwaitFocus()
				fsm.EventFocusAcquired()
				sc := fakegame.DefaultScenario()
				for range 3 {
					for _, phase := range sc.Timeline {
						if phase.Phase == fakegame.PhaseSplash {
							cues = append(cues, time.Now())
						}
						frames.set(phase.Phase)
						time.Sleep(time.Duration(phase.MS) * time.Millisecond)
					}
				}
				synctest.Wait()
				if len(presses) != len(cues) {
					t.Fatalf("got %d reel presses for %d scripted bites", len(presses), len(cues))
				}
				var delays []time.Duration
				for i, cue := range cues {
					delay := presses[i].Sub(cue)
					if delay < 0 || delay > time.Second {
						t.Fatalf("bite %d took %v", i, delay)
					}
					delays = append(delays, delay)
				}
				t.Logf("matched %d/%d scripted bites; cue-to-press %v; pointer steps %d", len(presses), len(cues), delays, moves)
			})
		})
	}
}
