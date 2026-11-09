package fishing

import (
	"context"
	"errors"
	"image"
	"log/slog"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/soocke/pixel-bot-go/assets"
	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/capture"
	"github.com/soocke/pixel-bot-go/domain/detect"
	"github.com/soocke/pixel-bot-go/internal/fakegame"
)

var testLogger = slog.New(slog.DiscardHandler)

// sceneFrames renders the fake game at a fixed screen position.
type sceneFrames struct {
	render  func(fakegame.Phase) *image.RGBA
	phase   atomic.Pointer[fakegame.Phase]
	origin  image.Point
	grabbed atomic.Pointer[image.Rectangle] // the last captured area
}

func (s *sceneFrames) set(p fakegame.Phase) { s.phase.Store(&p) }

// Window is where the scene sits on the screen.
func (s *sceneFrames) Window() (image.Rectangle, error) {
	return s.render(*s.phase.Load()).Bounds().Add(s.origin), nil
}

// Grab captures area of the scene, the way the screen would, and records it.
func (s *sceneFrames) Grab(area image.Rectangle) (capture.Frame, error) {
	s.grabbed.Store(&area)
	img := s.render(*s.phase.Load())
	sub := img.SubImage(area.Sub(s.origin)).(*image.RGBA)
	sub.Rect = sub.Rect.Sub(sub.Rect.Min) // a capture's pixels start at (0, 0)
	return capture.Frame{Image: sub, Origin: area.Min, CapturedAt: time.Now()}, nil
}

func startEngine(t *testing.T, engine *Engine) {
	t.Helper()
	if err := engine.Start(); err != nil {
		t.Fatal(err)
	}
}

func newSceneEngine(t *testing.T, actions ActionCallbacks) (*Engine, *FishingFSM, *sceneFrames, image.Rectangle) {
	t.Helper()
	tmpl, err := assets.FishingTargetImage()
	if err != nil {
		t.Fatal(err)
	}
	r := fakegame.NewRenderer(fakegame.DefaultScenario(), tmpl)
	cfg := testConfig()
	frames := &sceneFrames{render: r.Render, origin: image.Pt(-1000, 200)}
	frames.set(fakegame.PhaseBobber)
	fsm := NewFSM(nil, cfg, actions)
	detectors := func(config.Config) (detect.Locator, detect.BiteDetector, error) {
		return detect.NewMarkerLocator(), detect.NewMotionBiteDetector(nil), nil
	}
	engine := NewEngine(frames, noSound, fsm, detectors, cfg, testLogger)
	return engine, fsm, frames, r.BobberRect().Add(frames.origin)
}

// The whole pipeline without Tk: rendered frames, real matcher, real detector,
// real FSM, recorded input.
func TestEngineReelsWhenTheSceneSplashes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := &actionLog{}
		engine, fsm, frames, bobber := newSceneEngine(t, recordingActions(log))
		defer fsm.Close()
		startEngine(t, engine)
		defer engine.Stop()
		fsm.EventAwaitFocus()
		fsm.EventFocusAcquired()

		time.Sleep(500 * time.Millisecond)
		wantState(t, fsm, StateMonitoring)
		x, y, ok := fsm.TargetCoordinates()
		if !ok || !image.Pt(x, y).In(bobber) {
			t.Fatalf("target = (%d,%d) ok=%v, want it inside %v", x, y, ok, bobber)
		}
		// Watching captures only the box around the bobber; the preview keeps
		// the whole window and places the live box on it.
		time.Sleep(frameInterval)
		window, _ := frames.Window()
		if got := *frames.grabbed.Load(); !image.Pt(x, y).In(got) || got.Dx()*got.Dy() > window.Dx()*window.Dy()/20 {
			t.Fatalf("watching grabbed %v of window %v, want a small box around (%d,%d)", got, window, x, y)
		}
		if p := engine.Preview(); p.ROI == nil || p.Frame.Bounds().Size() != window.Size() || !image.Pt(x, y).Sub(window.Min).In(p.ROIBox) {
			t.Fatalf("preview frame %v, ROI box %v; want the window and the box at the bobber", p.Frame.Bounds(), p.ROIBox)
		}

		frames.set(fakegame.PhaseSplash)
		time.Sleep(200*time.Millisecond + reelClickDelay)

		wantState(t, fsm, StateCooldown)
		wantActions(t, log, "move", "move", "click")
	})
}

func TestInteractKeyReelRequiresAFreshMarkerAndReleasesOnFocusLoss(t *testing.T) {
	for _, test := range []struct {
		name       string
		loseMarker bool
		loseFocus  bool
	}{
		{name: "confirmed"},
		{name: "marker lost", loseMarker: true},
		{name: "focus lost during hold", loseFocus: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				log := &actionLog{}
				actions := recordingActions(log)
				actions.ParseVK = func(key string) (byte, bool) {
					return 'F', key == "F"
				}
				actions.HoldInteract = func(vk byte) func() {
					if vk != 'F' {
						t.Errorf("Interact vk = %d, want F", vk)
					}
					log.add("interact down")
					return func() { log.add("interact up") }
				}
				engine, fsm, frames, _ := newSceneEngine(t, actions)
				engine.cfg.ReelMethod, engine.cfg.InteractKey = config.ReelInteractKey, "F"
				defer fsm.Close()
				startEngine(t, engine)
				defer engine.Stop()
				fsm.EventAwaitFocus()
				fsm.EventFocusAcquired()
				time.Sleep(500 * time.Millisecond)
				wantState(t, fsm, StateMonitoring)
				wantActions(t, log)

				frames.set(fakegame.PhaseSplash)
				time.Sleep(frameInterval)
				wantState(t, fsm, StateReeling)
				if test.loseMarker {
					frames.set(fakegame.PhaseEmpty)
				}
				time.Sleep(reelClickDelay)
				if test.loseMarker {
					wantState(t, fsm, StateSearching)
					wantActions(t, log)
					return
				}
				wantActions(t, log, "interact down")
				if test.loseFocus {
					fsm.EventFocusLost()
					wantState(t, fsm, StateWaitingFocus)
					wantActions(t, log, "interact down", "interact up")
					return
				}
				frames.set(fakegame.PhaseEmpty) // reeling can immediately remove the marker
				time.Sleep(interactHold)
				wantState(t, fsm, StateCooldown)
				wantActions(t, log, "interact down", "interact up")
			})
		})
	}
}

func TestEngineStaysQuietOnAStaticBobber(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := &actionLog{}
		engine, fsm, _, _ := newSceneEngine(t, recordingActions(log))
		defer fsm.Close()
		startEngine(t, engine)
		defer engine.Stop()
		fsm.EventAwaitFocus()
		fsm.EventFocusAcquired()

		time.Sleep(2 * time.Second)

		wantState(t, fsm, StateMonitoring)
		wantActions(t, log, "move")
	})
}

func TestEngineRoutesCastAndReelToESP32(t *testing.T) {
	for _, test := range []struct {
		name       string
		failReel   bool
		rightClick bool
	}{
		{name: "both acknowledged"},
		{name: "reel failed", failReel: true},
		{name: "right click", rightClick: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				log := &actionLog{}
				actions := recordingActions(log)
				actions.ParseVK = func(key string) (byte, bool) {
					switch key {
					case "F3":
						return 0x72, true
					case "F":
						return 'F', true
					default:
						return 0, false
					}
				}
				actions.OpenKeyboard = func(run config.Config) (func(), error) {
					if run.ESP32Port != "COM3" {
						t.Errorf("opened port %q", run.ESP32Port)
					}
					log.add("open")
					return func() { log.add("close") }, nil
				}
				actions.TapKeyboard = func(vk byte, _ time.Duration) error {
					switch vk {
					case 0x72:
						log.add("cast")
					case 'F':
						log.add("reel")
						if test.failReel {
							return errors.New("lost acknowledgment")
						}
					default:
						t.Errorf("unexpected hardware key %d", vk)
					}
					return nil
				}
				engine, fsm, frames, _ := newSceneEngine(t, actions)
				engine.cfg.ReelMethod, engine.cfg.InteractKey, engine.cfg.ESP32Port = config.ReelInteractKey, "F", "COM3"
				if test.rightClick {
					// The app routes this move and click to the board through action.Guard.
					engine.cfg.ReelMethod, engine.cfg.InteractKey = config.ReelRightClick, ""
				}
				defer fsm.Close()
				startEngine(t, engine)
				defer engine.Stop()
				fsm.EventAwaitFocus()
				fsm.EventFocusAcquired()
				wantState(t, fsm, StateSearching)
				fsm.ForceCast()
				time.Sleep(500 * time.Millisecond)
				wantState(t, fsm, StateMonitoring)
				frames.set(fakegame.PhaseSplash)
				time.Sleep(frameInterval + reelClickDelay)
				if test.failReel {
					wantState(t, fsm, StateHalt)
					if fsm.Stats().Reels != 0 {
						t.Fatal("counted an unacknowledged reel")
					}
				} else {
					wantState(t, fsm, StateCooldown)
					if fsm.Stats().Reels != 1 {
						t.Fatal("acknowledged reel was not counted")
					}
				}
				engine.Stop()
				if test.rightClick {
					wantActions(t, log, "open", "cast", "move", "move", "click", "close")
				} else {
					wantActions(t, log, "open", "cast", "reel", "close")
				}
			})
		})
	}
}

func TestEnginePublishesPreviewUntilStopped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		engine, fsm, _, _ := newSceneEngine(t, recordingActions(&actionLog{}))
		defer fsm.Close()
		if engine.Preview() != nil {
			t.Fatal("preview before Start")
		}

		startEngine(t, engine)
		startEngine(t, engine) // a second Start is a no-op
		time.Sleep(100 * time.Millisecond)
		if p := engine.Preview(); p == nil || p.Frame == nil {
			t.Fatalf("preview = %+v, want a frame", p)
		}

		engine.Stop()
		engine.Stop()
		if engine.Preview() != nil {
			t.Error("preview survived Stop")
		}
	})
}

func TestEngineReportsDetectorsItCannotBuild(t *testing.T) {
	cfg := testConfig()
	fsm := NewFSM(nil, cfg, recordingActions(&actionLog{}))
	defer fsm.Close()
	engine := NewEngine(&failingFrames{}, noSound, fsm, func(config.Config) (detect.Locator, detect.BiteDetector, error) {
		return nil, nil, errors.New("unknown locator")
	}, cfg, testLogger)

	if err := engine.Start(); err == nil {
		t.Fatal("want an error")
	}
	engine.Stop() // nothing started, so this must be safe
}

type failingFrames struct{ calls atomic.Int32 }

func (f *failingFrames) Window() (image.Rectangle, error) {
	f.calls.Add(1)
	return image.Rectangle{}, errors.New("no window")
}

func (f *failingFrames) Grab(image.Rectangle) (capture.Frame, error) {
	return capture.Frame{}, errors.New("no screen")
}

func TestEngineSurvivesCaptureFailures(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		frames := &failingFrames{}
		cfg := testConfig()
		fsm := NewFSM(nil, cfg, recordingActions(&actionLog{}))
		defer fsm.Close()
		detectors := func(config.Config) (detect.Locator, detect.BiteDetector, error) {
			return nil, detect.NewMotionBiteDetector(nil), nil
		}
		engine := NewEngine(frames, noSound, fsm, detectors, cfg, testLogger)

		startEngine(t, engine)
		time.Sleep(time.Second)
		engine.Stop()

		if frames.calls.Load() < 10 {
			t.Errorf("calls = %d, want the loop to keep trying", frames.calls.Load())
		}
	})
}

// noSound is a SoundSource for runs that must not listen.
func noSound(context.Context, func([]byte, time.Time) error) error {
	panic("the engine listened for a detector that does not hear")
}

// earDetector reports a bite on the first loud packet it hears.
type earDetector struct{ detect.BiteDetector }

func (earDetector) Hear(pcm []byte, _ time.Time) bool { return pcm[0] != 0 }

func TestEngineReelsWhenTheDetectorHearsABite(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := &actionLog{}
		engine, fsm, _, _ := newSceneEngine(t, recordingActions(log))
		defer fsm.Close()
		bang := make(chan struct{})
		engine.detectors = func(config.Config) (detect.Locator, detect.BiteDetector, error) {
			return detect.NewMarkerLocator(), earDetector{detect.NewMotionBiteDetector(nil)}, nil
		}
		engine.sound = func(ctx context.Context, emit func([]byte, time.Time) error) error {
			quiet := []byte{0}
			for {
				select {
				case <-ctx.Done():
					return nil
				case <-bang:
					bang = nil // a closed channel stays ready
					emit([]byte{1}, time.Now())
				case <-time.After(10 * time.Millisecond):
					emit(quiet, time.Now())
				}
			}
		}
		startEngine(t, engine)
		defer engine.Stop()
		fsm.EventAwaitFocus()
		fsm.EventFocusAcquired()
		time.Sleep(500 * time.Millisecond)
		wantState(t, fsm, StateMonitoring)

		close(bang)
		time.Sleep(100*time.Millisecond + reelClickDelay)

		wantState(t, fsm, StateCooldown)
		wantActions(t, log, "move", "move", "click")
	})
}

// TestFakeGameESP32CameraRecovery gives the scene a fishable view only after
// reversing the first drag. An obstructed scene never produces a bobber.
func TestFakeGameESP32CameraRecovery(t *testing.T) {
	for _, tc := range []struct {
		name    string
		blocked bool
	}{
		{name: "reverse drag finds water"},
		{name: "water remains obstructed", blocked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var frames *sceneFrames
				var drags []int
				var steps []byte
				var heading, trialCasts, reels int
				var firstMove time.Time
				actions := ActionCallbacks{
					ParseVK: func(key string) (byte, bool) {
						switch key {
						case "F3":
							return 0x72, true
						case "F10":
							return 0x79, true
						}
						if len(key) == 1 {
							return key[0], true
						}
						return 0, false
					},
					OpenKeyboard: func(config.Config) (func(), error) {
						return func() {}, nil
					},
					PressKey:   func(byte) bool { t.Error("used Windows keyboard input"); return false },
					MoveCursor: func(int, int) { t.Error("used Windows cursor input") },
					DragCamera: func(int) (bool, bool) {
						t.Error("used a Windows camera drag")
						return true, false
					},
					DragHardwareCamera: func(dx int) (bool, bool) {
						if firstMove.IsZero() {
							firstMove = time.Now()
						}
						drags = append(drags, dx)
						heading += dx
						return true, true
					},
					TapKeyboard: func(vk byte, _ time.Duration) error {
						switch vk {
						case 'F':
							reels++
							frames.set(fakegame.PhaseEmpty)
						case 0x72:
							if reels != 0 {
								trialCasts++
							}
							if reels == 0 || !tc.blocked && len(drags) != 0 && heading == 0 {
								// Simulate the cast animation before the new marker appears.
								time.AfterFunc(200*time.Millisecond, func() { frames.set(fakegame.PhaseBobber) })
							}
						case 'W', 'A', 'S', 'D':
							if firstMove.IsZero() {
								firstMove = time.Now()
							}
							steps = append(steps, vk)
						default:
							t.Errorf("unexpected ESP32 key: %x", vk)
						}
						return nil
					},
				}
				engine, fsm, scene, _ := newSceneEngine(t, actions)
				frames = scene
				render := frames.render
				rendered := map[fakegame.Phase]*image.RGBA{}
				for _, phase := range []fakegame.Phase{fakegame.PhaseEmpty, fakegame.PhaseBobber, fakegame.PhaseSplash} {
					rendered[phase] = render(phase)
				}
				frames.render = func(phase fakegame.Phase) *image.RGBA { return rendered[phase] }
				frames.set(fakegame.PhaseEmpty)
				engine.cfg.ReelMethod = config.ReelInteractKey
				engine.cfg.InteractKey = "F"
				engine.cfg.ESP32Port = "COM3"
				engine.cfg.RepositionKey = config.RepositionAuto

				defer fsm.Close()
				startEngine(t, engine)
				defer engine.Stop()
				fsm.EventAwaitFocus()
				fsm.EventFocusAcquired()
				wantState(t, fsm, StateSearching)
				// Exercise recovery immediately. The FSM tests cover the normal
				// 45–55 reel trigger and the retry budget.
				fsm.nextReposition = 1
				fsm.ForceCast()
				time.Sleep(500 * time.Millisecond)
				wantState(t, fsm, StateMonitoring)
				frames.set(fakegame.PhaseSplash)
				time.Sleep(500 * time.Millisecond)
				wantState(t, fsm, StateCooldown)
				if fsm.Stats().Reels != 1 {
					t.Fatal("the fake game did not produce the initial catch")
				}
				for range 400 {
					time.Sleep(100 * time.Millisecond)
					if fsm.Current() == StateMonitoring || fsm.Current() == StateHalt {
						break
					}
				}
				synctest.Wait()
				if firstMove.IsZero() {
					t.Fatal("the fake game received no reposition input")
				}
				t.Logf("test casts=%d, WASD=%q, drags=%v, simulated recovery=%s",
					trialCasts, steps, drags, time.Since(firstMove))
				if !slices.Equal(drags, []int{12, -12}) || heading != 0 {
					t.Fatalf("expected opposite ESP32 drags, got %v at heading %d", drags, heading)
				}
				if tc.blocked {
					wantState(t, fsm, StateHalt)
					if trialCasts < 4 || trialCasts > 6 || !fsm.TakeRepositionLogout() || fsm.TakeRepositionLogout() {
						t.Fatalf("expected one logout request after 4–6 attempts, got %d attempts", trialCasts)
					}
					return
				}
				wantState(t, fsm, StateMonitoring)
				if trialCasts < 2 || trialCasts > 4 || fsm.TakeRepositionLogout() {
					t.Fatalf("expected recovery within four attempts, got %d attempts", trialCasts)
				}
				// The returned bobber must be usable, not just visible.
				time.Sleep(300 * time.Millisecond)
				frames.set(fakegame.PhaseSplash)
				time.Sleep(500 * time.Millisecond)
				wantState(t, fsm, StateCooldown)
				if fsm.Stats().Reels != 2 {
					t.Fatal("fishing did not resume after the reverse drag")
				}
			})
		})
	}
}
