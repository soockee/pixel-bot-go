package app

import (
	"context"
	"errors"
	"image"
	"io"
	"log/slog"
	"math/rand/v2"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/domain/capture"
	"github.com/soocke/pixel-bot-go/domain/detect"
	"github.com/soocke/pixel-bot-go/domain/fishing"
	"github.com/soocke/pixel-bot-go/internal/cursormotion"
	"github.com/soocke/pixel-bot-go/internal/esp32"
	"github.com/soocke/pixel-bot-go/internal/inputtiming"
	"github.com/soocke/pixel-bot-go/internal/playtrace"
	"github.com/soocke/pixel-bot-go/internal/recorder"
	"github.com/soocke/pixel-bot-go/internal/traceobserver"
	"github.com/soocke/pixel-bot-go/ui/desktop"
	"github.com/soocke/pixel-bot-go/ui/model"
	"github.com/soocke/pixel-bot-go/ui/presenter"
)

// AppContainer holds the components the app talks to after wiring.
type AppContainer struct {
	Config           *config.Config
	Capture          *model.CaptureModel
	FSM              *fishing.FishingFSM
	Engine           *fishing.Engine
	View             *desktop.View
	Frames           *desktop.Frames
	Recordings       *desktop.Recordings
	CapturePresenter *presenter.CapturePresenter
	RecordPresenter  *presenter.RecordPresenter
	Loop             *presenter.Loop
	trace            *liveTrace
	gameWindow       atomic.Pointer[capture.WindowIdentity]
}

// TraceOptions configures desktop tracing and the optional reel timing profile.
type TraceOptions struct {
	Model       *traceobserver.Model
	Output      *playtrace.Writer
	Start       time.Time
	Timing      func() inputtiming.Sample
	Calibration *inputtiming.Calibration
	Cursor      cursormotion.Planner
}

// BuildContainer constructs and wires all components. It opens no window;
// emit sends the window state to the frontend.
func BuildContainer(cfg *config.Config, logger *slog.Logger, trace TraceOptions, emit func(desktop.State)) *AppContainer {
	c := &AppContainer{Config: cfg, Capture: &model.CaptureModel{}, Frames: desktop.NewFrames()}
	c.View = desktop.NewView(c.Frames, emit)
	c.Recordings = desktop.NewRecordings(func() string { return cfg.RecordDir })
	inFront := c.gameFocused

	callbacks := fishing.ActionCallbacks{ParseVK: action.ParseVK, CanAct: inFront}
	dryRun := cfg.DryRun
	if dryRun {
		logger.Warn("dry-run enabled: input actions are logged, not injected")
		callbacks.PressKey, callbacks.MoveCursor, callbacks.ClickRight = action.DryRun(logger)
		callbacks.HoldInteract = func(vk byte) func() {
			if !inFront() {
				return nil
			}
			callbacks.PressKey(vk)
			return func() {}
		}
		if trace.Model != nil || trace.Output != nil {
			c.trace = newLiveTrace(trace.Model, trace.Output, trace.Start)
			callbacks.ObserveCast, callbacks.ObserveReel = c.trace.cast, c.trace.bite
		}
		if trace.Output != nil || trace.Timing != nil {
			callbacks.CanSimulateReel = inFront
			if trace.Output != nil {
				callbacks.ObserveReelTiming = c.trace.reelTiming
			}
		}
		// A dry run plans the pointer like a live run, with the recorded
		// profile or the default path, so the fake game shows real movement.
		plan := trace.Cursor
		if plan == nil {
			plan = cursormotion.DefaultPath
		}
		callbacks.Cursor = c.dryCursor(plan, inFront, callbacks.MoveCursor)
		callbacks.Approach = cursormotion.ApproachPoint
	} else {
		callbacks.ReelRoll = rand.IntN
		callbacks.RepositionRoll = rand.IntN
		var keyboard esp32.Keyboard
		// hardware routes the guard's pointer input to the board for the run.
		var hardware atomic.Bool
		callbacks.OpenKeyboard = func(run config.Config) (func(), error) {
			port, err := esp32.PortName(run.ESP32Port)
			if err != nil {
				return nil, err
			}
			keys := []string{run.CastKey}
			if run.ReelMethod == config.ReelInteractKey {
				keys = append(keys, run.InteractKey)
			}
			for _, key := range keys {
				vk, valid := action.ParseVK(key)
				if !valid || !esp32.SupportsKey(vk) {
					return nil, errors.New("ESP32 requires a letter, digit, F1–F12, Mouse 4 or Mouse 5 for Cast and Interact")
				}
			}
			if err := keyboard.Open(port); err != nil {
				return nil, err
			}
			if trace.Timing != nil {
				if err := keyboard.PrepareTimedTaps(); err != nil {
					return nil, err
				}
			}
			if run.Reposition || run.IdleMoves {
				if err := keyboard.PrepareCamera(); err != nil {
					return nil, err
				}
			}
			if err := keyboard.PrepareButtons(); err != nil {
				return nil, err
			}
			if run.ReelMethod != config.ReelInteractKey {
				if err := keyboard.PreparePointer(); err != nil {
					return nil, err
				}
				if !keyboard.CanPoint() {
					keyboard.Close()
					return nil, errors.New("flash the current ESP32 firmware to reel with a right click")
				}
			}
			logger.Info("ESP32 keyboard ready", "port", keyboard.Name(), "camera", keyboard.CanDragCamera(), "pointer", keyboard.CanPoint())
			hardware.Store(true)
			return func() {
				hardware.Store(false)
				keyboard.Close()
			}, nil
		}
		move := func(x, y int) bool {
			if !hardware.Load() {
				return action.MoveCursor(x, y)
			}
			if err := keyboard.MoveTo(image.Pt(x, y), action.CursorPos); err != nil {
				logger.Error("ESP32 pointer move failed", "error", err)
				return false
			}
			return true
		}
		click := func() bool {
			if !hardware.Load() {
				return action.ClickRight()
			}
			if err := keyboard.ClickRight(); err != nil {
				logger.Error("ESP32 right click failed", "error", err)
				return false
			}
			return true
		}
		callbacks.TapKeyboard = func(vk byte, hold time.Duration) error {
			if !inFront() {
				return errors.New("selected game window lost focus")
			}
			return keyboard.Press(vk, hold)
		}
		guard := &action.Guard{
			Logger: logger, InFront: inFront, Area: c.gameArea, CursorPos: action.CursorPos,
			// The guard runs on the FSM goroutine, which must not wait on its own queue.
			OnTakeover: func() { go c.FSM.EventHaltBecause("the mouse was moved") },
			Press:      action.PressKey, Move: move, Click: click,
			BeginHold: action.BeginRightHold, BeginKeyHold: action.BeginKeyHold,
		}
		callbacks.PressKey, callbacks.MoveCursor, callbacks.ClickRight = guard.PressKey, guard.MoveCursor, guard.ClickRight
		callbacks.CursorPos = action.CursorPos
		callbacks.HoldInteract = guard.HoldKey
		callbacks.HoldRight = guard.HoldRight
		callbacks.DragCamera = func(dx int) (bool, bool) { return guard.DragCamera(dx, action.DragCamera) }
		callbacks.DragHardwareCamera = func(dx int) (bool, bool) {
			if !keyboard.CanDragCamera() || action.LeftMouseDown() {
				return false, false
			}
			return guard.DragCamera(dx, func(dx int, allowed func() bool) bool {
				if !allowed() || action.LeftMouseDown() {
					return false
				}
				if err := keyboard.DragCamera(dx); err != nil {
					logger.Error("ESP32 camera drag failed", "error", err)
					return false
				}
				return true
			})
		}
		plan := trace.Cursor
		if plan == nil {
			plan = cursormotion.DefaultPath
		}
		callbacks.Approach = func(center image.Point, box image.Rectangle) (image.Point, bool) {
			area := c.gameArea()
			x, y, ok := action.CursorPos()
			if area == nil || !ok || !image.Pt(x, y).In(*area) {
				return center, false
			}
			return cursormotion.ApproachPoint(center, box.Intersect(*area))
		}
		callbacks.Cursor = &fishing.CursorCallbacks{
			Plan: func(target image.Point, settle bool) ([]fishing.CursorStep, bool) {
				start, area, ok := guard.BeginCursor()
				if !ok {
					return nil, false
				}
				return cursorSteps(plan(start, target, area, settle))
			},
			Move: guard.TryMoveCursor, Reset: guard.ResetCursor,
		}
	}
	if !dryRun || trace.Timing != nil || trace.Output != nil {
		callbacks.PlanReel = func(detector string) fishing.ReelTiming {
			var s inputtiming.Sample
			if dryRun {
				s.HoldUS = inputtiming.FixedHoldUS
			}
			if trace.Timing != nil {
				s = trace.Timing()
			}
			timing := fishing.ReelTiming{
				Hold:     time.Duration(s.HoldUS) * time.Microsecond,
				Cooldown: time.Duration(s.CooldownUS) * time.Microsecond,
			}
			if dryRun {
				timing.AfterDetection = time.Duration(trace.Calibration.DelayUS(detector, s)) * time.Microsecond
			} else {
				timing.AfterDetection = time.Duration(200+rand.IntN(121)+rand.IntN(121)) * time.Millisecond
			}
			return timing
		}
	}
	c.FSM = fishing.NewFSM(logger, cfg, callbacks)

	detectors := func(run config.Config) (detect.Locator, detect.BiteDetector, error) {
		locator, err := detect.NewLocator(run.Locator, run)
		if err != nil {
			return nil, nil, err
		}
		bite, err := detect.NewBiteDetector(run.BiteDetector, logger)
		return locator, bite, err
	}
	sound := func(ctx context.Context, emit func([]byte, time.Time) error) error {
		target := c.gameWindow.Load()
		if target == nil || !target.Valid() {
			return errors.New("selected game window is no longer available")
		}
		return capture.CaptureAudio(ctx, target.PID, emit)
	}
	c.Engine = fishing.NewEngine(c.frames(), sound, c.FSM, detectors, cfg, logger)

	c.CapturePresenter = presenter.NewCapturePresenter(c.Capture, c.Engine, c.FSM, c.View)
	c.RecordPresenter = presenter.NewRecordPresenter(c.prepareRecording, c.View)
	focusWatcher := presenter.NewFocusWatcher(c.FSM, inFront)
	c.FSM.AddListener(func(prev, next fishing.FishingState) {
		if dryRun && trace.Output != nil && (next == fishing.StateHalt || next == fishing.StateWaitingFocus) {
			c.trace.timingGap(next)
		}
		focusWatcher.OnState(prev, next)
		c.CapturePresenter.OnState(prev, next)
	})
	c.Loop = presenter.NewLoop(
		presenter.NewSessionPresenter(model.NewSessionModel(), c.Capture, c.View),
		presenter.NewFSMPresenter(c.FSM, c.View),
		c.CapturePresenter,
		c.RecordPresenter,
		presenter.NewPreviewPresenter(c.Engine, c.View),
	)
	return c
}

func (c *AppContainer) dryCursor(plan cursormotion.Planner, inFront func() bool, move func(int, int)) *fishing.CursorCallbacks {
	var current image.Point
	var positioned bool
	return &fishing.CursorCallbacks{
		Plan: func(target image.Point, settle bool) ([]fishing.CursorStep, bool) {
			area := c.gameArea()
			if !inFront() || area == nil {
				return nil, false
			}
			if !positioned || !current.In(*area) {
				current = area.Min.Add(area.Size().Div(2))
				positioned = true
			}
			return cursorSteps(plan(current, target, *area, settle))
		},
		Move: func(p image.Point) bool {
			area := c.gameArea()
			if !inFront() || area == nil || !p.In(*area) {
				return false
			}
			move(p.X, p.Y)
			current = p
			if c.trace != nil && c.trace.output != nil {
				c.trace.cursorWaypoint(p, *area)
			}
			return true
		},
		Reset: func() { positioned = false },
	}
}

func cursorSteps(path []cursormotion.Step) ([]fishing.CursorStep, bool) {
	steps := make([]fishing.CursorStep, len(path))
	for i, step := range path {
		steps[i] = fishing.CursorStep{After: step.After, Point: step.Point}
	}
	return steps, len(steps) > 0
}

// gameFrames captures the game window the person picked.
type gameFrames struct {
	target func() *capture.WindowIdentity
}

func (g gameFrames) Window() (image.Rectangle, error) {
	if w := g.target(); w != nil {
		return w.Area()
	}
	return image.Rectangle{}, errors.New("select a game window")
}
func (g gameFrames) Grab(area image.Rectangle) (capture.Frame, error) {
	return capture.Grab(area)
}

func (c *AppContainer) frames() gameFrames { return gameFrames{target: c.gameWindow.Load} }

// gameArea is the game window's client area, or nil when it is not open. The
// guard keeps every cursor move inside it.
func (c *AppContainer) gameArea() *image.Rectangle {
	area, err := c.frames().Window()
	if err != nil {
		return nil
	}
	return &area
}

// bindGameWindow resolves the current choice before a run. Every operation
// during that run uses this identity, including a scheduled logout.
func (c *AppContainer) bindGameWindow() error {
	if c.FSM.Current() != fishing.StateHalt {
		return errors.New("wait for the previous fishing run to stop")
	}
	target, err := capture.ResolveWindow(c.Capture.Window())
	if err != nil {
		return err
	}
	c.gameWindow.Store(&target)
	return nil
}

// startProblem explains why a run or recording must not start, or returns "".
func (c *AppContainer) startProblem() string {
	area, err := c.frames().Window()
	if err != nil {
		return err.Error()
	}
	// A test run configured for one monitor must never capture or click on another.
	if screen, ok := action.ScreenBounds(c.Config.Screen); ok && !area.In(screen) {
		return "the game window is outside the configured screen"
	}
	return ""
}

// prepareRecording builds a recording of the game window in a new timestamped
// folder.
func (c *AppContainer) prepareRecording() (presenter.RecordJob, error) {
	if err := c.bindGameWindow(); err != nil {
		return presenter.RecordJob{}, err
	}
	if problem := c.startProblem(); problem != "" {
		return presenter.RecordJob{}, errors.New(problem)
	}
	// A recording keeps the area it started with, so frame pixels and the saved
	// window offset mean the same thing for the whole session.
	target := *c.gameWindow.Load()
	region, err := target.Area()
	if err != nil {
		return presenter.RecordJob{}, err
	}
	session := recorder.Session{
		Dir:    filepath.Join(c.Config.RecordDir, time.Now().Format("20060102-150405")),
		Frames: capture.Source{Region: func() *image.Rectangle { return &region }},
		Window: c.Capture.Window(),
		Target: &target,
		Live:   true,
		Keys:   c.Config.RecordKeys,
		Status: io.Discard,
	}
	return presenter.RecordJob{Dir: session.Dir, Run: session.Run}, nil
}
