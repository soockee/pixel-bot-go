package fishing

import (
	"errors"
	"image"
	"log/slog"
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
	render func(fakegame.Phase) *image.RGBA
	phase  atomic.Pointer[fakegame.Phase]
	origin image.Point
}

func (s *sceneFrames) set(p fakegame.Phase) { s.phase.Store(&p) }
func (s *sceneFrames) Next() (capture.Frame, error) {
	img := s.render(*s.phase.Load())
	return capture.Frame{Image: img, Origin: s.origin, CapturedAt: time.Now()}, nil
}

func startEngine(t *testing.T, engine *Engine) {
	t.Helper()
	if err := engine.Start(); err != nil {
		t.Fatal(err)
	}
}

func newSceneEngine(t *testing.T, log *actionLog) (*Engine, *FishingFSM, *sceneFrames, image.Rectangle) {
	t.Helper()
	tmpl, err := assets.FishingTargetImage()
	if err != nil {
		t.Fatal(err)
	}
	r := fakegame.NewRenderer(fakegame.DefaultScenario(), tmpl)
	cfg := testConfig()
	frames := &sceneFrames{render: r.Render, origin: image.Pt(-1000, 200)}
	frames.set(fakegame.PhaseBobber)
	fsm := NewFSM(nil, cfg, recordingActions(log))
	detectors := func(config.Config) (detect.Locator, detect.BiteDetector, error) {
		return detect.NewMarkerLocator(), detect.NewMotionBiteDetector(nil), nil
	}
	engine := NewEngine(frames, fsm, detectors, cfg, testLogger)
	return engine, fsm, frames, r.BobberRect().Add(frames.origin)
}

// The whole pipeline without Tk: rendered frames, real matcher, real detector,
// real FSM, recorded input.
func TestEngineReelsWhenTheSceneSplashes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := &actionLog{}
		engine, fsm, frames, bobber := newSceneEngine(t, log)
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

		frames.set(fakegame.PhaseSplash)
		time.Sleep(200*time.Millisecond + reelClickDelay)

		wantState(t, fsm, StateCooldown)
		wantActions(t, log, "move", "move", "click")
	})
}

func TestEngineStaysQuietOnAStaticBobber(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := &actionLog{}
		engine, fsm, _, _ := newSceneEngine(t, log)
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

func TestEnginePublishesPreviewUntilStopped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		engine, fsm, _, _ := newSceneEngine(t, &actionLog{})
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
	engine := NewEngine(&failingFrames{}, fsm, func(config.Config) (detect.Locator, detect.BiteDetector, error) {
		return nil, nil, errors.New("unknown locator")
	}, cfg, testLogger)

	if err := engine.Start(); err == nil {
		t.Fatal("want an error")
	}
	engine.Stop() // nothing started, so this must be safe
}

type failingFrames struct{ calls atomic.Int32 }

func (f *failingFrames) Next() (capture.Frame, error) {
	f.calls.Add(1)
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
		engine := NewEngine(frames, fsm, detectors, cfg, testLogger)

		startEngine(t, engine)
		time.Sleep(time.Second)
		engine.Stop()

		if frames.calls.Load() < 10 {
			t.Errorf("calls = %d, want the loop to keep trying", frames.calls.Load())
		}
	})
}
