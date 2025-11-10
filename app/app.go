package app

import (
	"fmt"
	"image"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/domain/fishing"
	"github.com/soocke/pixel-bot-go/ui/presenter"
	"github.com/soocke/pixel-bot-go/ui/view"

	//lint:ignore ST1001 Dot import is intentional for concise Tk widget DSL builders.
	. "modernc.org/tk9.0"
)

// Adapters removed; app now injects concrete services directly (CaptureSvc, SelectionOverlay, UI).

const (
	tick = 100 * time.Millisecond
)

type app struct {
	container      *AppContainer
	logger         *slog.Logger
	configPath     string
	width, height  int
	start          time.Time
	afterID        string
	selectedWindow string
	loop           *presenter.Loop
	goWg           sync.WaitGroup
	selectionView  view.SelectionOverlay
	shutdown       atomic.Bool // indicates graceful shutdown initiated
}

// Inline convenience getters reduce surface area; presenters now depend directly on container services.
func (a *app) captureRunning() bool {
	return a.container.CaptureSvc != nil && a.container.CaptureSvc.Running()
}
func (a *app) captureFrames() <-chan *image.RGBA {
	if a.container.CaptureSvc != nil {
		return a.container.CaptureSvc.Frames()
	}
	return nil
}
func (a *app) selectionRect() *image.Rectangle {
	if a.selectionView != nil {
		return a.selectionView.ActiveRect()
	}
	return nil
}

func NewApp(title string, width, height int, cfg *config.Config, logger *slog.Logger) *app {
	container := BuildContainer(cfg, logger, width, height, "pixle_bot_config.json")
	a := &app{container: container, logger: logger, configPath: "pixle_bot_config.json", width: width, height: height}

	App.WmTitle(title)
	WmProtocol(App, "WM_DELETE_WINDOW", a.exitHandler)
	WmGeometry(App, fmt.Sprintf("%dx%d+100+100", width, height))
	a.selectionView = view.NewSelectionOverlay(cfg, a.configPath, logger)
	return a
}

// detectViewAdapter removed; RootView (UI) already satisfies DetectionView interface.

// Run builds layout, wires presenters, starts update loop and blocks until exit.
func (a *app) Run() (err error) {
	cfg := a.container.Config
	a.layout()
	a.container.RootView.SetConfigEditable(true)
	// Wire presenters now that UI is ready.
	a.container.SessionPresenter = presenter.NewSessionPresenter(a.container.Session, a.container.Capture, a.container.UI)
	a.container.FSMPresenter = presenter.NewFSMPresenter(a.container.FSM, a.container.UI)
	a.container.DetectionPresenter = presenter.NewDetectionPresenter(
		func() bool { return a.container.Capture.Enabled() },
		a.container.CaptureSvc,
		a.container.FSM,
		a.selectionView,
		a.container.UI,
		cfg,
		a.container.TargetImg,
		a.container.Detection,
	)
	a.container.CapturePresenter = presenter.NewCapturePresenter(a.container.Capture, a.container.CaptureSvc, a.container.FSM, a.container.RootView)

	// Focus watcher starts only while FSM awaits focus; not part of main Loop ticks.
	focusWatcher := presenter.NewFocusWatcher(a.container.FSM, a.logger, nil, func() string { return strings.TrimSpace(strings.ToLower(a.selectedWindow)) })
	a.loop = presenter.NewLoop(a.container.SessionPresenter, a.container.FSMPresenter, a.container.DetectionPresenter, a.ScheduleUpdate)

	// Record start time and schedule first tick.
	a.start = time.Now()
	// Forward FSM state changes to presenter.
	a.container.FSM.AddListener(func(prev, next fishing.FishingState) {
		if a.container.FSMPresenter != nil {
			a.container.FSMPresenter.OnState(next)
		}
		focusWatcher.OnState(prev, next)
	})

	a.ScheduleUpdate()
	App.Wait()

	a.goWg.Wait()
	return nil
}

func (a *app) layout() {
	var titles []string
	if list, err := action.ListWindows(); err == nil {
		titles = list
	}
	rv := a.container.RootView
	rv.Build(titles, func() { a.toggleCapture() }, func() {
		if a.selectionView != nil {
			a.selectionView.OpenOrFocus()
		}
	}, a.exitHandler, func(title string) { a.selectedWindow = title })
	a.container.UI = rv
	// After view & selection overlay are ready, attach selection provider to capture service.
	if a.container.CaptureSvc != nil {
		if selView := a.selectionView; selView != nil {
			a.container.CaptureSvc.SetSelectionProvider(func() *image.Rectangle { return selView.ActiveRect() })
		}
	}
}

func (a *app) exitHandler() {
	if a.afterID != "" {
		TclAfterCancel(a.afterID)
	}
	if a.container.FSM != nil {
		a.container.FSM.Close()
	}
	Destroy(App)
}

func (a *app) ScheduleUpdate() {
	a.afterID = TclAfter(tick, func() {
		if a.loop != nil {
			a.loop.Tick()
		}
	})
}

// getSelectionRect proxies to selectionView (may be nil).
// toggleCapture delegates to presenter.
func (a *app) toggleCapture() {
	if cp := a.container.CapturePresenter; cp != nil {
		cp.Toggle()
	}
}

// safeGo removed; inline goroutines use explicit panic recovery where needed.

// (legacy preview/state methods removed in favor of presenters)
func (a *app) Running() bool                   { return a.captureRunning() }
func (a *app) Frames() <-chan *image.RGBA      { return a.captureFrames() }
func (a *app) SelectionRect() *image.Rectangle { return a.selectionRect() }

// DetectionView methods already via ui: UpdateCapture, UpdateDetection
// DetectionFSM adapter methods
// Current returns current fishing state (legacy adapter method retained for compatibility elsewhere).
func (a *app) Current() fishing.FishingState {
	if a.container.FSM != nil {
		return a.container.FSM.Current()
	}
	return fishing.StateHalt
}
func (a *app) EventTargetAcquiredAt(x, y int) {
	if a.container.FSM != nil {
		a.container.FSM.EventTargetAcquiredAt(x, y)
	}
}
func (a *app) TargetCoordinates() (int, int, bool) {
	if a.container.FSM != nil {
		return a.container.FSM.TargetCoordinates()
	}
	return 0, 0, false
}
func (a *app) ProcessMonitoringFrame(img *image.RGBA, now time.Time) {
	if a.container.FSM != nil {
		a.container.FSM.ProcessMonitoringFrame(img, now)
	}
}
