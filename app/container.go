package app

import (
	"context"
	"errors"
	"image"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/domain/capture"
	"github.com/soocke/pixel-bot-go/domain/detect"
	"github.com/soocke/pixel-bot-go/domain/fishing"
	"github.com/soocke/pixel-bot-go/internal/cursormotion"
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
	inFront := func() bool {
		title, err := action.ForegroundWindowTitle()
		return err == nil && title != "" && strings.EqualFold(title, strings.TrimSpace(c.Capture.Window()))
	}

	callbacks := fishing.ActionCallbacks{ParseVK: action.ParseVK}
	dryRun := cfg.DryRun
	if dryRun {
		logger.Warn("dry-run enabled: input actions are logged, not injected")
		callbacks.PressKey, callbacks.MoveCursor, callbacks.ClickRight = action.DryRun(logger)
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
		if trace.Cursor != nil {
			callbacks.Cursor = c.dryCursor(trace.Cursor, inFront, callbacks.MoveCursor)
		}
	} else {
		guard := &action.Guard{
			Logger: logger, InFront: inFront, Area: c.gameArea, CursorPos: action.CursorPos,
			// The guard runs on the FSM goroutine, which must not wait on its own queue.
			OnTakeover: func() { go c.FSM.EventHalt() },
			Press:      action.PressKey, Move: action.MoveCursor, Click: action.ClickRight,
			BeginHold: action.BeginRightHold,
		}
		callbacks.PressKey, callbacks.MoveCursor, callbacks.ClickRight = guard.PressKey, guard.MoveCursor, guard.ClickRight
		if trace.Timing != nil {
			callbacks.HoldRight = guard.HoldRight
		}
		if trace.Cursor != nil {
			callbacks.Cursor = &fishing.CursorCallbacks{
				Plan: func(target image.Point, settle bool) ([]fishing.CursorStep, bool) {
					start, area, ok := guard.BeginCursor()
					if !ok {
						return nil, false
					}
					return cursorSteps(trace.Cursor(start, target, area, settle))
				},
				Move: guard.TryMoveCursor, Reset: guard.ResetCursor,
			}
		}
	}
	if trace.Timing != nil || dryRun && trace.Output != nil {
		callbacks.PlanReel = func(detector string) fishing.ReelTiming {
			s := inputtiming.Sample{HoldUS: inputtiming.FixedHoldUS}
			if trace.Timing != nil {
				s = trace.Timing()
			}
			timing := fishing.ReelTiming{
				Hold:     time.Duration(s.HoldUS) * time.Microsecond,
				Cooldown: time.Duration(s.CooldownUS) * time.Microsecond,
			}
			if dryRun {
				timing.AfterDetection = time.Duration(trace.Calibration.DelayUS(detector, s)) * time.Microsecond
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
		pid, err := capture.WindowProcess(c.Capture.Window())
		if err != nil {
			return err
		}
		return capture.CaptureAudio(ctx, pid, emit)
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
type gameFrames struct{ title func() string }

func (g gameFrames) Window() (image.Rectangle, error) { return capture.WindowArea(g.title()) }
func (g gameFrames) Grab(area image.Rectangle) (capture.Frame, error) {
	return capture.Grab(area)
}

func (c *AppContainer) frames() gameFrames { return gameFrames{title: c.Capture.Window} }

// gameArea is the game window's client area, or nil when it is not open. The
// guard keeps every cursor move inside it.
func (c *AppContainer) gameArea() *image.Rectangle {
	area, err := capture.WindowArea(c.Capture.Window())
	if err != nil {
		return nil
	}
	return &area
}

// startProblem explains why a run or recording must not start, or returns "".
func (c *AppContainer) startProblem() string {
	area, err := capture.WindowArea(c.Capture.Window())
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
	if problem := c.startProblem(); problem != "" {
		return presenter.RecordJob{}, errors.New(problem)
	}
	// A recording keeps the area it started with, so frame pixels and the saved
	// window offset mean the same thing for the whole session.
	region, err := capture.WindowArea(c.Capture.Window())
	if err != nil {
		return presenter.RecordJob{}, err
	}
	session := recorder.Session{
		Dir:    filepath.Join(c.Config.RecordDir, time.Now().Format("20060102-150405")),
		Frames: capture.Source{Region: func() *image.Rectangle { return &region }},
		Window: c.Capture.Window(),
		Live:   true,
		Keys:   c.Config.RecordKeys,
		Status: io.Discard,
	}
	return presenter.RecordJob{Dir: session.Dir, Run: session.Run}, nil
}
