package app

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/debug"
	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/ui/theme"
	"github.com/soocke/pixel-bot-go/ui/view"

	//lint:ignore ST1001 Dot import is intentional for concise Tk widget DSL builders.
	. "modernc.org/tk9.0"
)

const tick = 33 * time.Millisecond

type app struct {
	container *AppContainer
	logger    *slog.Logger
	start     time.Time
	afterID   string
}

// NewApp creates the main window and all components.
func NewApp(title string, width, height int, cfg *config.Config, logger *slog.Logger, trace TraceOptions) (*app, error) {
	container, err := BuildContainer(cfg, logger, trace)
	if err != nil {
		return nil, err
	}
	a := &app{container: container, logger: logger}

	App.WmTitle(title)
	WmProtocol(App, "WM_DELETE_WINDOW", a.exitHandler)
	originX, originY := 0, 0
	if b, ok := action.ScreenBounds(cfg.Screen); ok {
		originX, originY = b.Min.X, b.Min.Y
	} else if cfg.Screen > 0 {
		logger.Warn("configured screen not found; using default placement", "screen", cfg.Screen)
	}
	WmGeometry(App, fmt.Sprintf("%dx%d+%d+%d", width, height, originX+100, originY+100))
	theme.InitStyles()
	return a, nil
}

// Run builds the UI, starts the update tick and blocks until the window is closed.
func (a *app) Run() error {
	c := a.container
	cfg := c.Config
	if cfg.AutoWindow != "" {
		c.Capture.SetWindow(cfg.AutoWindow)
	}
	c.RootView.Build(view.RootCallbacks{
		ListWindows: func() []string {
			titles, err := action.ListWindows()
			if err != nil {
				a.logger.Error("list windows failed", "error", err)
			}
			return titles
		},
		Window:        c.Capture.Window,
		SetWindow:     c.Capture.SetWindow,
		ToggleCapture: a.toggleCapture,
		OpenSelection: c.Selection.OpenOrFocus,
		Exit:          a.exitHandler,
	})
	a.watchStopKey()

	a.start = time.Now()
	if cfg.Debug || a.hasTraceObserver() {
		debug.RegisterState(a.stateSnapshot)
	}
	if cfg.AutoStart {
		a.logger.Info("auto-start requested", "window", c.Capture.Window())
		TclAfter(500*time.Millisecond, a.toggleCapture)
	}
	a.scheduleTick()
	App.Wait()
	return nil
}

func (a *app) exitHandler() {
	if a.afterID != "" {
		TclAfterCancel(a.afterID)
	}
	a.container.Engine.Stop()
	a.container.FSM.Close()
	Destroy(App)
}

func (a *app) scheduleTick() {
	a.afterID = TclAfter(tick, func() {
		a.container.Loop.Tick()
		a.scheduleTick()
	})
}

// startProblem explains why capture must not start, or returns "".
func (a *app) startProblem() string {
	// A false match outside a chosen area would click whatever is there.
	sel := a.container.Selection.ActiveRect()
	if sel == nil {
		return "no saved selection"
	}
	// A test run configured for one monitor must never capture or click on another.
	if screen, ok := action.ScreenBounds(a.container.Config.Screen); ok && !sel.In(screen) {
		return "selection is outside the configured screen"
	}
	return ""
}

// toggleCapture delegates to the capture presenter after checking that a start is safe.
func (a *app) toggleCapture() {
	if !a.container.Capture.Enabled() {
		if problem := a.startProblem(); problem != "" {
			a.logger.Error("start refused: " + problem)
			a.container.RootView.SetStatus("Cannot start: " + problem)
			return
		}
	}
	a.container.RootView.SetStatus("Ready")
	a.container.CapturePresenter.Toggle()
}

// watchStopKey registers the global stop hotkey. It goes straight to the FSM so
// it still works if Tk stalls; the capture presenter then stops capture.
func (a *app) watchStopKey() {
	key := a.container.Config.StopKey
	vk, ok := action.ParseVK(key)
	if !ok {
		a.logger.Error("unknown stop key", "key", key)
		a.container.RootView.SetStatus("Stop hotkey unavailable: unknown key " + key)
		return
	}
	if err := action.WatchHotkey(vk, a.container.FSM.EventHalt); err != nil {
		a.logger.Error("stop hotkey unavailable", "key", key, "error", err)
		a.container.RootView.SetStatus("Stop hotkey unavailable: " + key + " is in use")
	}
}

func (a *app) hasTraceObserver() bool {
	return a.container.trace != nil && a.container.trace.observer != nil
}

// stateSnapshot reports live state for the /debug/state endpoint (debug mode only).
func (a *app) stateSnapshot() map[string]any {
	c := a.container
	snap := map[string]any{
		"fsm_state":       c.FSM.Current().String(),
		"capture_running": c.Capture.Enabled(),
		"selected_window": c.Capture.Window(),
		"uptime_seconds":  time.Since(a.start).Seconds(),
	}
	if a.hasTraceObserver() {
		snap["trace_observer"] = c.trace.snapshot()
	}
	if x, y, ok := c.FSM.TargetCoordinates(); ok {
		snap["target"] = map[string]int{"x": x, "y": y}
	}
	return snap
}
