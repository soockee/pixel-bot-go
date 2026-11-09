package fishing

import (
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
			actions := recordingActions(log)
			actions.Cursor = &CursorCallbacks{
				Plan: func(target image.Point, settle bool) ([]CursorStep, bool) {
					if settle {
						return []CursorStep{{After: time.Second, Point: target}}, true
					}
					return []CursorStep{
						{After: 200 * time.Millisecond, Point: target.Sub(image.Pt(5, 0))},
						{After: 600 * time.Millisecond, Point: target},
					}, true
				},
				Move: func(p image.Point) bool {
					if p == image.Pt(10, 10) {
						log.add("target")
					} else {
						log.add("waypoint")
					}
					return true
				},
				Reset: func() {},
			}
			fsm := NewFSM(nil, testConfig(), actions)
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

func TestRefusedCursorStepHaltsBeforeReel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := &actionLog{}
		actions := recordingActions(log)
		actions.Cursor = &CursorCallbacks{
			Plan: func(target image.Point, _ bool) ([]CursorStep, bool) {
				return []CursorStep{{After: 100 * time.Millisecond, Point: target}}, true
			},
			Move:  func(image.Point) bool { return false },
			Reset: func() {},
		}
		fsm := NewFSM(nil, testConfig(), actions)
		defer fsm.Close()
		startMonitoring(t, fsm)
		fsm.EventFishBite()
		time.Sleep(reelClickDelay + time.Second)
		wantState(t, fsm, StateHalt)
		wantActions(t, log)
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
				actions := recordingActions(&actionLog{})
				var fsm *FishingFSM
				var presses, cues []time.Time
				var moves int
				actions.ClickRight = func() bool {
					x, y, ok := fsm.TargetCoordinates()
					if !ok || position != image.Pt(x, y) {
						t.Errorf("reel attempted at %v, target is (%d,%d)", position, x, y)
					}
					presses = append(presses, time.Now())
					return true
				}
				actions.MoveCursor = func(x, y int) {
					position = image.Pt(x, y)
					moves++
				}
				if profile == "fitted" {
					planner := model.Planner(42)
					actions.Cursor = &CursorCallbacks{
						Plan: func(target image.Point, settle bool) ([]CursorStep, bool) {
							path := planner(position, target, bounds, settle)
							steps := make([]CursorStep, len(path))
							for i, s := range path {
								steps[i] = CursorStep{After: s.After, Point: s.Point}
							}
							return steps, len(steps) > 0
						},
						Move: func(p image.Point) bool {
							actions.MoveCursor(p.X, p.Y)
							return p.In(bounds)
						},
						Reset: func() {},
					}
				}
				engine, runFSM, frames, _ := newSceneEngine(t, actions)
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
