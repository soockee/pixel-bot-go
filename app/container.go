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
	Selection        *desktop.Selection
	Recordings       *desktop.Recordings
	CapturePresenter *presenter.CapturePresenter
	RecordPresenter  *presenter.RecordPresenter
	Loop             *presenter.Loop
	trace            *liveTrace
}

// TraceOptions configures optional observation of desktop dry-run callbacks.
type TraceOptions struct {
	Model  *traceobserver.Model
	Output *playtrace.Writer
	Start  time.Time
}

// BuildContainer constructs and wires all components. It opens no window;
// emit sends the window state to the frontend.
func BuildContainer(cfg *config.Config, logger *slog.Logger, trace TraceOptions, emit func(desktop.State)) *AppContainer {
	c := &AppContainer{Config: cfg, Capture: &model.CaptureModel{}, Frames: desktop.NewFrames()}
	c.View = desktop.NewView(c.Frames, emit)
	c.Selection = desktop.NewSelection(cfg, config.FilePath, logger)
	c.Recordings = desktop.NewRecordings(func() string { return cfg.RecordDir })
	inFront := func() bool {
		title, err := action.ForegroundWindowTitle()
		return err == nil && title != "" && strings.EqualFold(title, strings.TrimSpace(c.Capture.Window()))
	}

	callbacks := fishing.ActionCallbacks{ParseVK: action.ParseVK}
	if cfg.DryRun {
		logger.Warn("dry-run enabled: input actions are logged, not injected")
		callbacks.PressKey, callbacks.MoveCursor, callbacks.ClickRight = action.DryRun(logger)
		if trace.Model != nil || trace.Output != nil {
			c.trace = newLiveTrace(trace.Model, trace.Output, trace.Start)
			callbacks.ObserveCast, callbacks.ObserveReel = c.trace.cast, c.trace.bite
		}
	} else {
		guard := &action.Guard{
			Logger: logger, InFront: inFront, Area: c.Selection.ActiveRect, CursorPos: action.CursorPos,
			// The guard runs on the FSM goroutine, which must not wait on its own queue.
			OnTakeover: func() { go c.FSM.EventHalt() },
			Press:      action.PressKey, Move: action.MoveCursor, Click: action.ClickRight,
		}
		callbacks.PressKey, callbacks.MoveCursor, callbacks.ClickRight = guard.PressKey, guard.MoveCursor, guard.ClickRight
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

func (c *AppContainer) frames() capture.Source {
	return capture.Source{Region: c.Selection.ActiveRect}
}

// startProblem explains why a run or recording must not start, or returns "".
func (c *AppContainer) startProblem() string {
	// A false match outside a chosen area would click whatever is there.
	sel := c.Selection.ActiveRect()
	if sel == nil {
		return "no saved selection"
	}
	// A test run configured for one monitor must never capture or click on another.
	if screen, ok := action.ScreenBounds(c.Config.Screen); ok && !sel.In(screen) {
		return "selection is outside the configured screen"
	}
	return ""
}

// prepareRecording builds a recording of the selected window and selection in
// a new timestamped folder.
func (c *AppContainer) prepareRecording() (presenter.RecordJob, error) {
	if problem := c.startProblem(); problem != "" {
		return presenter.RecordJob{}, errors.New(problem)
	}
	// A recording keeps the area it started with, so frame pixels and the saved
	// window offset mean the same thing for the whole session.
	region := *c.Selection.ActiveRect()
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
