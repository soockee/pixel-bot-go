package fishing

import (
	"os"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/soocke/pixel-bot-go/internal/fakegame"
	"github.com/soocke/pixel-bot-go/internal/inputtiming"
)

func TestSimulatedReelTiming(t *testing.T) {
	for _, stop := range []string{"", "halt", "focus", "searching"} {
		t.Run("interrupt="+stop, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				log := &actionLog{}
				actions := recordingActions(log)
				actions.PlanReel = func(string) ReelTiming {
					return ReelTiming{Hold: 120 * time.Millisecond, Cooldown: 2 * time.Second}
				}
				var events []ReelTimingEvent
				actions.ObserveReelTiming = func(e ReelTimingEvent) { events = append(events, e) }
				fsm := NewFSM(nil, testConfig(), actions)
				defer fsm.Close()
				var valid atomic.Bool
				valid.Store(true)
				fsm.EventAwaitFocus()
				fsm.EventFocusAcquired()
				fsm.send(evtTargetAcquiredAt{x: 10, y: 10, valid: valid.Load})
				fsm.EventFishBite()
				synctest.Wait()
				wantState(t, fsm, StateReeling)
				switch stop {
				case "halt":
					fsm.EventHalt()
				case "focus":
					fsm.EventFocusLost()
				case "searching":
					valid.Store(false) // the deadline must revalidate before dispatch
				}
				time.Sleep(reelClickDelay)
				synctest.Wait()
				if stop != "" {
					wantActions(t, log, "move", "move")
					if len(events) != 2 || events[1].Kind != "reel_canceled" || events[1].Reason != stop {
						t.Fatalf("unexpected cancellation: %+v", events)
					}
					return
				}
				wantState(t, fsm, StateReeling)
				wantActions(t, log, "move", "move", "click")
				time.Sleep(120 * time.Millisecond)
				synctest.Wait()
				wantState(t, fsm, StateCooldown)
				if len(events) != 2 || events[1].Kind != "reel_released" {
					t.Fatalf("unexpected release: %+v", events)
				}
				time.Sleep(2 * time.Second)
				synctest.Wait()
				wantActions(t, log, "move", "move", "click", "press")
			})
		})
	}
}

func TestTimedInputAlwaysReleasesOnInterruption(t *testing.T) {
	for _, interruption := range []string{"stop", "focus", "geometry", "shutdown"} {
		t.Run(interruption, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				log := &actionLog{}
				actions := recordingActions(log)
				actions.PlanReel = func(string) ReelTiming { return ReelTiming{Hold: 140 * time.Millisecond} }
				actions.HoldRight = func() func() {
					log.add("down")
					return func() { log.add("up") }
				}
				fsm := NewFSM(nil, testConfig(), actions)
				defer fsm.Close()
				var valid atomic.Bool
				valid.Store(true)
				fsm.EventAwaitFocus()
				fsm.EventFocusAcquired()
				fsm.send(evtTargetAcquiredAt{x: 10, y: 10, valid: valid.Load})
				fsm.EventFishBite()
				synctest.Wait()
				time.Sleep(reelClickDelay)
				wantActions(t, log, "move", "move", "down")
				switch interruption {
				case "stop":
					fsm.EventHalt()
				case "focus":
					fsm.EventFocusLost()
				case "geometry":
					valid.Store(false)
					fsm.send(evtTargetInvalidated{})
				case "shutdown":
					fsm.Close()
				}
				wantActions(t, log, "move", "move", "down", "up")
				time.Sleep(200 * time.Millisecond)
				wantActions(t, log, "move", "move", "down", "up")
			})
		})
	}
}

func TestCalibratedDelayIncludesTimeAlreadySpentAfterDetection(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := &actionLog{}
		actions := recordingActions(log)
		actions.PlanReel = func(string) ReelTiming {
			return ReelTiming{Hold: 120 * time.Millisecond, AfterDetection: 400 * time.Millisecond}
		}
		fsm := NewFSM(nil, testConfig(), actions)
		defer fsm.Close()
		startMonitoring(t, fsm)
		fsm.send(evtFishBite{at: time.Now().Add(-150 * time.Millisecond)})
		synctest.Wait()
		time.Sleep(249 * time.Millisecond)
		wantActions(t, log, "move", "move")
		time.Sleep(time.Millisecond)
		wantActions(t, log, "move", "move", "click")
	})
}

func TestPairedTimingReplay(t *testing.T) {
	model := inputtiming.Model{Samples: []inputtiming.Sample{
		{HoldUS: 120_000, CooldownUS: 800_000},
		{HoldUS: 145_000, CooldownUS: 1_200_000},
	}}
	if path := os.Getenv("VRL_TIMING_REPLAY_MODEL"); path != "" {
		var err error
		model, err = inputtiming.Load(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, profile := range []string{"fixed", "fitted"} {
		t.Run(profile, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var cues, presses, releases []time.Time
				var holds []time.Duration
				sample := model.Sampler(42)
				actions := recordingActions(&actionLog{})
				actions.ClickRight = func() bool { presses = append(presses, time.Now()); return true }
				actions.PlanReel = func(detector string) ReelTiming {
					s := inputtiming.Sample{HoldUS: inputtiming.FixedHoldUS}
					if profile == "fitted" {
						s = sample()
					}
					hold := time.Duration(s.HoldUS) * time.Microsecond
					holds = append(holds, hold)
					return ReelTiming{
						Hold: hold, Cooldown: time.Duration(s.CooldownUS) * time.Microsecond,
						AfterDetection: time.Duration(model.Calibration.DelayUS(detector, s)) * time.Microsecond,
					}
				}
				actions.ObserveReelTiming = func(e ReelTimingEvent) {
					if e.Kind == "reel_released" {
						releases = append(releases, time.Now())
					}
				}
				engine, fsm, frames, _ := newSceneEngine(t, actions)
				engine.cfg.CooldownSeconds = 3
				defer fsm.Close()
				frames.set(fakegame.PhaseEmpty)
				startEngine(t, engine)
				defer engine.Stop()
				fsm.EventAwaitFocus()
				fsm.EventFocusAcquired()
				sc := fakegame.DefaultScenario()
				for range 3 {
					for _, step := range sc.Timeline {
						if step.Phase == fakegame.PhaseSplash {
							cues = append(cues, time.Now())
						}
						frames.set(step.Phase)
						time.Sleep(time.Duration(step.MS) * time.Millisecond)
					}
				}
				synctest.Wait()
				if len(presses) != len(cues) || len(releases) != len(cues) {
					t.Fatalf("%d scripted bites, %d presses, %d releases", len(cues), len(presses), len(releases))
				}
				var delays []time.Duration
				for i, cue := range cues {
					delay := presses[i].Sub(cue)
					if delay < 0 || delay > 1500*time.Millisecond || releases[i].Sub(presses[i]) != holds[i] {
						t.Fatalf("bite %d: delay %v, hold %v, want hold %v", i, delay, releases[i].Sub(presses[i]), holds[i])
					}
					delays = append(delays, delay)
				}
				t.Logf("matched %d/%d scripted bites; cue-to-press %v; simulated holds %v", len(presses), len(cues), delays, holds)
			})
		})
	}
}
