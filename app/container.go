package app

import (
	"context"
	"errors"
	"fmt"
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
	// botLog copies the bot's log into a recording that runs alongside it.
	botLog     *recorder.BotLog
	Loop       *presenter.Loop
	trace      *liveTrace
	gameWindow atomic.Pointer[capture.WindowIdentity]
	board      boardOwner
	input      fishing.Input // the run's, which a scheduled key uses too
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
	botLog := recorder.NewBotLog(logger.Handler())
	logger = slog.New(botLog) // everything built here logs through it
	c := &AppContainer{Config: cfg, Capture: &model.CaptureModel{}, Frames: desktop.NewFrames(), botLog: botLog}
	c.View = desktop.NewView(c.Frames, emit)
	c.Recordings = desktop.NewRecordings(func() string { return cfg.RecordDir })
	inFront := c.gameFocused

	callbacks := fishing.ActionCallbacks{InFront: inFront, Roll: rand.IntN, Observer: fishing.NopObserver{}}
	dryRun := cfg.DryRun
	openBoard := fishing.BoardOpener(func(config.Config) (func(), error) {
		return nil, errors.New("a dry run opens no Board")
	})
	if dryRun {
		logger.Warn("dry-run enabled: input actions are logged, not injected")
		if trace.Model != nil || trace.Output != nil {
			c.trace = newLiveTrace(trace.Model, trace.Output, trace.Start)
			callbacks.Observer = c.trace
		}
		// A dry run plans the pointer like a live run, with the recorded
		// profile or the default path, so the fake game shows real movement.
		plan := trace.Cursor
		if plan == nil {
			plan = cursormotion.DefaultPath
		}
		pointer := &dryPointer{c: c, inFront: inFront, plan: plan}
		callbacks.Pointer = pointer
		callbacks.Input = action.DryRun{Logger: logger, InFront: inFront, Area: c.gameArea, Moved: pointer.Moved}
		callbacks.SimulateHolds = true
	} else {
		callbacks.DeliberateMisses = true
		c.board.board.Logger = logger
		board := &c.board.board // usable only while a run holds it
		openBoard = func(run config.Config) (func(), error) {
			if run.ESP32Port == "" {
				return nil, errors.New("select or connect the ESP32 board in Settings")
			}
			port, err := esp32.PortName(run.ESP32Port)
			if err != nil {
				return nil, err
			}
			// Engine.Start checked the keys (config.CheckRunKeys); the Board
			// only needs to know whether its firmware presses Mouse 4 and 5.
			keys := []string{run.CastKey}
			if run.Schedule || run.Reposition {
				keys = append(keys, run.LogoutKey)
			}
			if run.ReelMethod == config.ReelInteractKey {
				keys = append(keys, run.InteractKey)
			}
			_, release, err := c.board.open(port)
			if err != nil {
				return nil, err
			}
			keepOpen := false
			defer func() {
				if !keepOpen {
					release()
				}
			}()
			if trace.Timing != nil {
				if err := board.PrepareTimedTaps(); err != nil {
					return nil, err
				}
			}
			if run.Reposition || run.IdleMoves {
				if err := board.PrepareCamera(); err != nil {
					return nil, err
				}
			}
			if run.ReelMethod != config.ReelInteractKey || run.IdleMoves {
				if err := board.PreparePointer(); err != nil {
					return nil, err
				}
			}
			if err := board.PrepareButtons(); err != nil {
				return nil, err
			}
			for _, key := range keys {
				vk, _ := config.KeyCode(key)
				if (vk == 0x05 || vk == 0x06) && !board.CanPressButtons() {
					return nil, errors.New("flash the current ESP32 firmware to press Mouse 4 and Mouse 5")
				}
			}
			if run.ReelMethod != config.ReelInteractKey {
				if !board.CanPoint() {
					return nil, errors.New("flash the current ESP32 firmware to reel with a right click")
				}
			}
			if run.IdleMoves && !board.CanPoint() {
				return nil, errors.New("flash the current ESP32 firmware to move the pointer")
			}
			// Only what this run asked the board: a capability it never asked reads false.
			ready := []any{"port", board.Name(), "buttons", board.CanPressButtons()}
			if run.Reposition || run.IdleMoves {
				ready = append(ready, "camera", board.CanDragCamera())
			}
			if run.ReelMethod != config.ReelInteractKey || run.IdleMoves {
				ready = append(ready, "pointer", board.CanPoint())
			}
			logger.Info("ESP32 keyboard ready", ready...)
			keepOpen = true
			return release, nil
		}
		move := func(x, y int) error {
			if err := board.MoveTo(image.Pt(x, y), action.CursorPos); err != nil {
				logger.Error("ESP32 pointer move failed", "error", err)
				if board.CanPoint() { // the port is still open: only this aim missed
					return fmt.Errorf("%w; see pixel_bot_logs.json", fishing.ErrPointerMissed)
				}
				return errors.New("the ESP32 could not put the pointer on the target; see pixel_bot_logs.json")
			}
			return nil
		}
		click := func() error {
			if err := board.ClickRight(); err != nil {
				logger.Error("ESP32 right click failed", "error", err)
				return errors.New("the ESP32 could not click; see pixel_bot_logs.json")
			}
			return nil
		}
		guard := &action.Guard{
			Logger: logger, InFront: inFront, Area: c.gameArea, CursorPos: action.CursorPos,
			// The guard runs on the FSM goroutine, which must not wait on its own queue.
			OnTakeover: func() { go c.FSM.EventHaltBecause("the mouse was moved") },
			Press:      board.Press, Move: move, Click: click,
			CanDrag: func() bool { return board.CanDragCamera() && !action.LeftMouseDown() },
			Drag:    board.DragCamera,
		}
		callbacks.Input = guard
		plan := trace.Cursor
		if plan == nil {
			plan = cursormotion.DefaultPath
		}
		callbacks.Pointer = livePointer{c: c, guard: guard, plan: plan}
	}
	// A dry run without a trace keeps every default.
	callbacks.PlanReel = func(string) fishing.ReelTiming { return fishing.ReelTiming{} }
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
	c.input = callbacks.Input
	c.FSM = fishing.NewFSM(logger, cfg, callbacks)

	detectors := func(config.Config) (detect.Locator, detect.BiteDetector, error) {
		return detect.NewMarkerLocator(), detect.NewSplashSoundDetector(logger), nil
	}
	sound := func(ctx context.Context, emit func([]byte, time.Time) error) error {
		target := c.gameWindow.Load()
		if target == nil || !target.Valid() {
			return errors.New("selected game window is no longer available")
		}
		return capture.CaptureAudio(ctx, target.PID, emit)
	}
	c.Engine = fishing.NewEngine(c.frames(), sound, c.FSM, detectors, openBoard, cfg, logger)

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

// gameFrames captures screen pixels within the selected game window.
type gameFrames struct {
	target func() *capture.WindowIdentity
}

func (g gameFrames) Window() (image.Rectangle, error) {
	if w := g.target(); w != nil {
		return w.Area()
	}
	return image.Rectangle{}, errors.New("select a game window")
}
func (gameFrames) Grab(area image.Rectangle) (capture.Frame, error) {
	return capture.Grab(area)
}

func (c *AppContainer) frames() gameFrames {
	return gameFrames{target: c.gameWindow.Load}
}

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
	// A run already holds the game window; a recording beside it uses the same.
	if c.FSM.Current() == fishing.StateHalt || c.gameWindow.Load() == nil {
		if err := c.bindGameWindow(); err != nil {
			return presenter.RecordJob{}, err
		}
	}
	if problem := c.startProblem(); problem != "" {
		return presenter.RecordJob{}, errors.New(problem)
	}
	target := *c.gameWindow.Load()
	dir := filepath.Join(c.Config.RecordDir, time.Now().Format("20060102-150405"))
	session := recorder.Session{
		Dir:     dir,
		Window:  c.Capture.Window(),
		Target:  &target,
		Live:    true,
		Keys:    c.Config.RecordKeys,
		Status:  io.Discard,
		OnStart: func(start time.Time) { c.botLog.Start(dir, start) },
	}
	run := func(ctx context.Context) error {
		defer c.botLog.Stop()
		return session.Run(ctx)
	}
	return presenter.RecordJob{Dir: session.Dir, Run: run}, nil
}
