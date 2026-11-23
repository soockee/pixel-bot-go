package fishing

import (
	"errors"
	"fmt"
	"image"
	"math/rand/v2"
	"slices"
	"strings"
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

// testInput is the Input of FSM tests: each method calls its field, and an
// unset field succeeds without doing anything.
type testInput struct {
	tap   func(vk byte, hold time.Duration) error
	move  func(p image.Point) error
	click func() error
	drag  func(dx int) (attempted bool, err error)
}

func (i *testInput) Tap(vk byte, hold time.Duration) error {
	if i.tap == nil {
		return nil
	}
	return i.tap(vk, hold)
}

func (i *testInput) MoveTo(p image.Point) error {
	if i.move == nil {
		return nil
	}
	return i.move(p)
}

func (i *testInput) ClickRight() error {
	if i.click == nil {
		return nil
	}
	return i.click()
}

func (i *testInput) DragCamera(dx int) (attempted bool, err error) {
	if i.drag == nil {
		return false, nil
	}
	return i.drag(dx)
}

// testPointer is the Pointer of FSM tests: each method calls its field, and
// an unset field goes straight to the target, waits on the bobber itself and
// does not know where the pointer is.
type testPointer struct {
	plan     func(target image.Point, settle bool) ([]CursorStep, error)
	reset    func()
	approach func(center image.Point, box image.Rectangle) (image.Point, bool)
}

func (p *testPointer) Plan(target image.Point, settle bool) ([]CursorStep, error) {
	if p.plan == nil {
		return []CursorStep{{Point: target}}, nil
	}
	return p.plan(target, settle)
}

func (p *testPointer) Reset() {
	if p.reset != nil {
		p.reset()
	}
}

func (p *testPointer) Approach(center image.Point, box image.Rectangle) (image.Point, bool) {
	if p.approach == nil {
		return center, true
	}
	return p.approach(center, box)
}

func (p *testPointer) Pos() (image.Point, bool) { return image.Point{}, false }

// pointerOf is the testPointer of deps made by recordingDeps.
func pointerOf(deps Deps) *testPointer { return deps.Pointer.(*testPointer) }

// reelTimings is an Observer that only collects reel timing events.
type reelTimings func(ReelTimingEvent)

func (reelTimings) Cast() func()                   { return func() {} }
func (reelTimings) Reel() func()                   { return func() {} }
func (o reelTimings) ReelTiming(e ReelTimingEvent) { o(e) }

// inputOf is the testInput of deps made by recordingDeps.
func inputOf(deps Deps) *testInput { return deps.Input.(*testInput) }

// settleAtRollZero is the wait after a reposition move when every Roll is 0.
const settleAtRollZero = 550 * time.Millisecond

func recordingDeps(log *actionLog) Deps {
	return Deps{
		Input: &testInput{
			tap:   func(byte, time.Duration) error { log.add("press"); return nil },
			move:  func(image.Point) error { log.add("move"); return nil },
			click: func() error { log.add("click"); return nil },
		},
		SimulateHolds: true, // testConfig has no Board
		InFront:       func() bool { return true },
		Pointer:       &testPointer{},
		Observer:      NopObserver{},
		PlanReel:      func(string) ReelTiming { return ReelTiming{} },
		Roll:          rand.New(rand.NewPCG(1, 2)).IntN,
	}
}

func newRecordingFSM(log *actionLog) *FishingFSM {
	return NewFSM(nil, testConfig(), recordingDeps(log))
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
		t.Fatalf("deps = %v, want %v", got, want)
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

// Once Bobber Marker shows outcomes, a cast waits for the reel's outcome, as a
// cast before the loot opens loses the catch; it casts anyway after outcomeWait.
func TestCooldownWaitsForTheOutcome(t *testing.T) {
	reel := func(t *testing.T, m *FishingFSM) {
		startMonitoring(t, m)
		m.EventOutcome(OutcomeNone) // the blocks show an earlier run's outcome
		m.EventFishBite()
		time.Sleep(reelClickDelay)
		wantState(t, m, StateCooldown)
	}
	fsmTest(t, func(t *testing.T, m *FishingFSM, _ *actionLog) {
		reel(t, m)
		time.Sleep(2 * time.Second)
		wantState(t, m, StateCooldown) // the 1 s pause is over, the loot is not open
		m.EventOutcome(OutcomeCaught)
		time.Sleep(time.Second - time.Millisecond)
		wantState(t, m, StateCooldown)
		time.Sleep(time.Millisecond)
		wantState(t, m, StateSearching)
		if s := m.Stats(); s.Caught != 1 || s.Reels != 1 {
			t.Fatalf("stats = %+v, want one reel and one catch", s)
		}
	})
	fsmTest(t, func(t *testing.T, m *FishingFSM, _ *actionLog) {
		reel(t, m)
		time.Sleep(outcomeWait - time.Millisecond)
		wantState(t, m, StateCooldown)
		time.Sleep(time.Millisecond)
		wantState(t, m, StateSearching)
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
		m := NewFSM(nil, cfg, recordingDeps(log))
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
		if got, want := m.Stats(), (Stats{Casts: 2, Reels: 1, ReelAttempts: 1, ReelInputs: 1}); got != want {
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

func TestSampledReelMiss(t *testing.T) {
	for _, limit := range []int{1, 2} {
		synctest.Test(t, func(t *testing.T) {
			cfg := testConfig()
			cfg.MaxFailedCasts = limit
			log := &actionLog{}
			deps := recordingDeps(log)
			samples := 0
			deps.DeliberateMisses = true
			deps.Roll = func(_ int) int {
				samples++
				if samples == 1 {
					return 0
				}
				return 2
			}
			m := NewFSM(nil, cfg, deps)
			defer m.Close()
			startMonitoring(t, m)
			m.EventFishBite()
			if limit == 1 {
				wantState(t, m, StateHalt)
			} else {
				wantState(t, m, StateCooldown)
			}
			wantActions(t, log, "move")
			if got := m.Stats(); got.Reels != 0 || got.FailedInRow != 1 {
				t.Fatalf("sampled miss produced stats %+v", got)
			}
			if limit == 1 {
				return
			}
			time.Sleep(time.Second)
			wantState(t, m, StateSearching)
			wantActions(t, log, "move", "press")
			m.EventTargetAcquiredAt(10, 10)
			wantState(t, m, StateMonitoring)
			m.EventFishBite()
			time.Sleep(reelClickDelay)
			wantState(t, m, StateCooldown)
			wantActions(t, log, "move", "press", "move", "move", "click")
			if got := m.Stats(); got.Reels != 1 || got.FailedInRow != 0 || samples != 2 {
				t.Fatalf("successful reel after a miss produced stats %+v and %d samples", got, samples)
			}
		})
	}
}

func TestOffTargetReel(t *testing.T) {
	for _, tc := range []struct {
		name                string
		interact, loseFocus bool
	}{
		{name: "click beside the bobber"},
		{name: "focus loss cancels the click", loseFocus: true},
		{name: "Interact falls back to a skip", interact: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				cfg := testConfig()
				if tc.interact {
					cfg.ReelMethod = config.ReelInteractKey
				}
				log := &actionLog{}
				deps := recordingDeps(log)
				deps.DeliberateMisses = true
				deps.Roll = func(int) int { return 1 }
				aim := image.Pt(52, 100)
				at := image.Point{}
				inputOf(deps).move = func(p image.Point) error { at = p; return nil }
				inputOf(deps).click = func() error {
					if at != aim {
						log.add("wrong click point")
					}
					log.add("click")
					return nil
				}
				m := NewFSM(nil, cfg, deps)
				defer m.Close()
				m.EventAwaitFocus()
				m.EventFocusAcquired()
				m.send(evtTargetAcquiredAt{x: 100, y: 100, missAim: &aim})
				wantState(t, m, StateMonitoring)
				m.EventFishBite()
				if tc.loseFocus {
					wantState(t, m, StateReeling)
					m.EventFocusLost()
					time.Sleep(reelClickDelay + interactHold)
					wantState(t, m, StateWaitingFocus)
					wantActions(t, log)
					return
				}
				if tc.interact {
					wantState(t, m, StateCooldown)
					wantActions(t, log)
				} else {
					wantState(t, m, StateReeling)
					time.Sleep(reelClickDelay + interactHold)
					wantState(t, m, StateCooldown)
					wantActions(t, log, "click")
				}
				if got := m.Stats(); got.Reels != 0 || got.FailedInRow != 1 {
					t.Fatalf("intentional miss produced stats %+v", got)
				}
			})
		})
	}
}

func TestRepositionChecksTheNextCast(t *testing.T) {
	for _, outcome := range []string{"fresh bobber", "recovered after retry", "no bobber", "focus lost"} {
		t.Run(outcome, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				cfg := testConfig()
				cfg.Reposition = true
				log := &actionLog{}
				deps := recordingDeps(log)
				deps.Roll = func(int) int { return 0 } // reposition after 45 reels, settle 550 ms
				inputOf(deps).tap = func(vk byte, _ time.Duration) error {
					if strings.ContainsRune("WASD", rune(vk)) {
						log.add("reposition")
					} else {
						log.add("cast")
					}
					return nil
				}
				m := NewFSM(nil, cfg, deps)
				defer m.Close()
				startMonitoring(t, m)
				threshold := m.nextReposition
				if threshold < 45 || threshold > 55 {
					t.Fatalf("reposition threshold = %d", threshold)
				}
				for reel := int32(1); reel <= threshold; reel++ {
					m.EventFishBite()
					time.Sleep(reelClickDelay + time.Second)
					if reel < threshold {
						wantState(t, m, StateSearching)
						m.EventTargetAcquiredAt(10, 10)
						wantState(t, m, StateMonitoring)
					}
				}
				wantState(t, m, StateCooldown)
				before := log.snapshot()
				if before[len(before)-1] != "reposition" || slices.Contains(before[:len(before)-1], "reposition") {
					t.Fatalf("unexpected reposition sequence: %v", before)
				}
				if outcome == "focus lost" {
					m.EventFocusLost()
					time.Sleep(time.Second)
					wantState(t, m, StateWaitingFocus) // paused, not stopped
					if !slices.Equal(log.snapshot(), before) {
						t.Fatal("sent input after focus loss")
					}
					m.EventFocusAcquired()
					wantState(t, m, StateSearching)
					m.EventTargetAcquiredAt(10, 10)
					wantState(t, m, StateMonitoring)
					if m.TakeRepositionLogout() {
						t.Fatal("the check after the pause asked to log out")
					}
					return
				}
				time.Sleep(interactHold + settleAtRollZero) // a dry run holds the key first
				wantState(t, m, StateSearching)
				wantActions(t, log, append(before, "cast")...)
				if outcome == "no bobber" || outcome == "recovered after retry" {
					limit := m.repositionLimit
					if limit < 4 || limit > 6 {
						t.Fatalf("reposition attempt limit = %d", limit)
					}
					for attempt := 1; attempt < limit; attempt++ {
						time.Sleep(searchTimeout)
						wantState(t, m, StateCooldown)
						time.Sleep(interactHold + settleAtRollZero)
						wantState(t, m, StateSearching)
						before = append(before, "cast", "reposition")
						wantActions(t, log, append(before, "cast")...)
						if outcome == "recovered after retry" {
							break
						}
					}
					if outcome == "no bobber" {
						time.Sleep(searchTimeout)
						wantState(t, m, StateHalt)
						if got, want := m.HaltReason(), fmt.Sprintf("no bobber found after %d reposition attempts", limit); got != want {
							t.Fatalf("halt reason = %q, want %q", got, want)
						}
						wantActions(t, log, append(before, "cast")...)
						if !m.TakeRepositionLogout() || m.TakeRepositionLogout() {
							t.Fatal("expected exactly one logout request")
						}
						return
					}
				}
				if m.TakeRepositionLogout() {
					t.Fatal("requested logout before exhausting attempts")
				}
				m.EventTargetAcquiredAt(10, 10)
				wantState(t, m, StateMonitoring)
				m.EventFishBite()
				time.Sleep(reelClickDelay)
				wantState(t, m, StateCooldown)
				if got := m.Stats().Reels; got != int(threshold)+1 {
					t.Fatalf("reels after returning to fishing = %d", got)
				}
			})
		})
	}
}

func TestAutomaticRepositionUsesOnlyESP32WhenSelected(t *testing.T) {
	var taps []byte
	f := &FishingFSM{
		cfg: config.Config{
			Reposition: true,
			ReelMethod: config.ReelInteractKey,
			ESP32Port:  "AUTO",
		},
		logger:              testLogger,
		repositionDirection: 4,
		deps: Deps{
			InFront: func() bool { return true },
			Input: &testInput{tap: func(vk byte, _ time.Duration) error {
				taps = append(taps, vk)
				return nil
			}},
		},
	}
	for f.repositionAttempt = 0; f.repositionAttempt < 4; f.repositionAttempt++ {
		if !f.moveForReposition() {
			t.Fatalf("attempt %d did not move", f.repositionAttempt)
		}
	}
	if string(taps) != "WSAD" {
		t.Fatalf("ESP32 taps = %q, want WSAD", taps)
	}
}

func TestAutomaticRepositionDragsThroughESP32(t *testing.T) {
	var drags []int
	f := &FishingFSM{
		cfg: config.Config{
			Reposition: true,
			ReelMethod: config.ReelInteractKey,
			ESP32Port:  "AUTO",
		},
		logger: testLogger,
		deps: Deps{
			InFront: func() bool { return true },
			Input: &testInput{drag: func(dx int) (bool, error) {
				drags = append(drags, dx)
				return true, nil
			}},
		},
	}
	for f.repositionAttempt = 0; f.repositionAttempt < 2; f.repositionAttempt++ {
		if !f.moveForReposition() {
			t.Fatalf("attempt %d did not move", f.repositionAttempt)
		}
	}
	if !slices.Equal(drags, []int{12, -12}) {
		t.Fatalf("ESP32 drags = %v", drags)
	}
}

func TestBlockedRightClickIsOnlyAnAttempt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		deps := recordingDeps(&actionLog{})
		inputOf(deps).click = func() error { return errors.New("the mouse was moved") }
		m := NewFSM(nil, testConfig(), deps)
		defer m.Close()
		startMonitoring(t, m)
		m.EventFishBite()
		time.Sleep(reelClickDelay)
		wantState(t, m, StateHalt)
		if got := m.Stats(); got.ReelAttempts != 1 || got.ReelInputs != 0 || got.Reels != 0 {
			t.Fatalf("blocked click changed the confirmed counts: %+v", got)
		}
		if got := m.HaltReason(); got != "could not reel: the mouse was moved" {
			t.Errorf("halt reason %q, want the click's own reason", got)
		}
	})
}

func TestApproachAndReelDelayAreIndependent(t *testing.T) {
	for _, moveBeforeBite := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			log := &actionLog{}
			deps := recordingDeps(log)
			pointerOf(deps).approach = func(_ image.Point, box image.Rectangle) (image.Point, bool) {
				return box.Min.Add(image.Pt(4, 4)), moveBeforeBite
			}
			inputOf(deps).move = func(p image.Point) error {
				log.add(fmt.Sprintf("move to %d,%d", p.X, p.Y))
				return nil
			}
			deps.PlanReel = func(string) ReelTiming {
				return ReelTiming{AfterDetection: 220 * time.Millisecond}
			}
			fsm := NewFSM(nil, testConfig(), deps)
			defer fsm.Close()
			fsm.EventAwaitFocus()
			fsm.EventFocusAcquired()
			fsm.send(evtTargetAcquiredAt{x: 10, y: 10, box: image.Rect(0, 0, 20, 20)})
			wantState(t, fsm, StateMonitoring)
			var want []string
			if moveBeforeBite {
				want = append(want, "move to 4,4")
			}
			wantActions(t, log, want...)
			fsm.EventFishBite()
			time.Sleep(219 * time.Millisecond)
			want = append(want, "move to 10,10")
			wantActions(t, log, want...)
			time.Sleep(time.Millisecond)
			wantState(t, fsm, StateCooldown)
			wantActions(t, log, append(want, "click")...)
		})
	}
}

// A dry run reels every bite: without DeliberateMisses, a roll of 0 is not a
// planned miss.
func TestNoDeliberateMissWithoutTheSwitch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := &actionLog{}
		deps := recordingDeps(log)
		deps.Roll = func(int) int { return 0 } // "forgot to reel" when misses are on
		m := NewFSM(nil, testConfig(), deps)
		defer m.Close()
		startMonitoring(t, m)
		m.EventFishBite()
		time.Sleep(reelClickDelay)
		wantState(t, m, StateCooldown)
		wantActions(t, log, "move", "move", "click")
	})
}
