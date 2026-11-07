package app

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/soocke/pixel-bot-go/assets"
	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/domain/capture"
	"github.com/soocke/pixel-bot-go/domain/detect"
	"github.com/soocke/pixel-bot-go/domain/fishing"
	"github.com/soocke/pixel-bot-go/internal/playtrace"
	"github.com/soocke/pixel-bot-go/internal/traceobserver"
	"github.com/soocke/pixel-bot-go/ui/model"
	"github.com/soocke/pixel-bot-go/ui/presenter"
	"github.com/soocke/pixel-bot-go/ui/view"
)

// AppContainer holds the components the app talks to after wiring.
type AppContainer struct {
	Config           *config.Config
	Capture          *model.CaptureModel
	FSM              *fishing.FishingFSM
	Engine           *fishing.Engine
	RootView         *view.RootView
	Selection        *view.SelectionOverlay
	CapturePresenter *presenter.CapturePresenter
	Loop             *presenter.Loop
	trace            *liveTrace
}

// TraceOptions configures optional observation of desktop dry-run callbacks.
type TraceOptions struct {
	Model  *traceobserver.Model
	Output *playtrace.Writer
	Start  time.Time
}

// BuildContainer constructs and wires all components. It creates no Tk widgets.
func BuildContainer(cfg *config.Config, logger *slog.Logger, trace TraceOptions) (*AppContainer, error) {
	tmpl, err := assets.FishingTargetImage()
	if err != nil {
		return nil, fmt.Errorf("load bobber template: %w", err)
	}
	c := &AppContainer{Config: cfg, Capture: &model.CaptureModel{}}
	c.RootView = view.NewRootView(cfg, config.FilePath, logger)
	c.Selection = view.NewSelectionOverlay(cfg, config.FilePath, logger)
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

	newLocator := func(run config.Config) detect.Locator { return detect.NewNCCLocator(tmpl, run) }
	frames := capture.Source{Region: c.Selection.ActiveRect}
	c.Engine = fishing.NewEngine(frames, c.FSM, newLocator, detect.NewMotionBiteDetector(logger), cfg, logger)

	c.CapturePresenter = presenter.NewCapturePresenter(c.Capture, c.Engine, c.FSM, c.RootView)
	focusWatcher := presenter.NewFocusWatcher(c.FSM, inFront)
	c.FSM.AddListener(func(prev, next fishing.FishingState) {
		focusWatcher.OnState(prev, next)
		c.CapturePresenter.OnState(prev, next)
	})
	c.Loop = presenter.NewLoop(
		presenter.NewSessionPresenter(model.NewSessionModel(), c.Capture, c.RootView),
		presenter.NewFSMPresenter(c.FSM, c.RootView),
		c.CapturePresenter,
		presenter.NewPreviewPresenter(c.Engine, c.RootView),
	)
	return c, nil
}
