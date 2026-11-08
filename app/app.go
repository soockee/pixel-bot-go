package app

import (
	"context"
	"image"
	"log/slog"
	"net/http"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/debug"
	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/frontend"
	"github.com/soocke/pixel-bot-go/ui/desktop"
)

const (
	tick         = 33 * time.Millisecond
	devtoolsPort = "9222" // WebView2 DevTools on localhost, debug mode only
)

func init() { application.RegisterEvent[desktop.State](desktop.StateEvent) }

type app struct {
	container *AppContainer
	logger    *slog.Logger
	start     time.Time
	loop      *desktop.Loop
	wails     *application.App
	main      *application.WebviewWindow
	overlay   *application.WebviewWindow // the selection overlay while it is open; loop only
}

// NewApp builds the components and the window.
func NewApp(title string, width, height int, cfg *config.Config, logger *slog.Logger, trace TraceOptions) *app {
	a := &app{logger: logger}
	a.container = BuildContainer(cfg, logger, trace, func(s desktop.State) { a.wails.Event.Emit(desktop.StateEvent, s) })
	c := a.container
	a.loop = desktop.NewLoop(a.tick)
	service := desktop.NewService(a.loop, c.View, desktop.Commands{
		ListWindows: func() []string {
			titles, err := action.ListWindows()
			if err != nil {
				logger.Error("list windows failed", "error", err)
			}
			return titles
		},
		SetWindow:        a.setWindow,
		ToggleCapture:    a.toggleCapture,
		ToggleRecord:     c.RecordPresenter.Toggle,
		OpenSelection:    a.openSelection,
		ConfirmSelection: a.confirmSelection,
		CloseSelection:   a.closeSelection,
		Running:          c.Capture.Enabled,
		Exit:             func() { go a.wails.Quit() }, // Quit waits for the loop, which runs this
	}, c.Selection, c.Recordings, cfg, config.FilePath, logger)

	assets := http.NewServeMux()
	assets.Handle(desktop.FramesPath, c.Frames)
	assets.Handle(desktop.RecordingsPath, c.Recordings)
	assets.Handle("/", application.BundledAssetFileServer(frontend.Dist()))
	opts := application.Options{
		Name:       title,
		Services:   []application.Service{application.NewService(service)},
		Assets:     application.AssetOptions{Handler: assets},
		OnShutdown: a.shutdown,
	}
	if cfg.Debug {
		// Lets agents drive and inspect the UI over the DevTools protocol.
		opts.Windows.AdditionalBrowserArgs = []string{"--remote-debugging-port=" + devtoolsPort}
	}
	a.wails = application.New(opts)
	a.main = a.wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "main", Title: title, Width: width, Height: height, MinWidth: 880, MinHeight: 600,
		BackgroundColour: application.NewRGB(0x12, 0x13, 0x14), URL: "/", Hidden: true,
	})
	a.main.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) { go a.wails.Quit() })
	// The native window exists only once the app runs, so it is placed then.
	a.wails.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		if b, ok := action.ScreenBounds(cfg.Screen); ok {
			p := a.main.PhysicalBounds()
			a.main.SetPhysicalBounds(application.Rect{X: b.Min.X + 100, Y: b.Min.Y + 100, Width: p.Width, Height: p.Height})
		} else if cfg.Screen > 0 {
			logger.Warn("configured screen not found; using default placement", "screen", cfg.Screen)
		}
		a.main.Show()
	})
	return a
}

// Run starts the UI loop and blocks until the app quits.
func (a *app) Run() error {
	c := a.container
	cfg := c.Config
	if cfg.AutoWindow != "" {
		a.setWindow(cfg.AutoWindow)
	}
	c.View.SetInfo(cfg.DryRun, cfg.StopKey, cfg.DarkMode)
	c.View.SetSelection(desktop.RectOf(c.Selection.ActiveRect()), false)
	c.View.SetStatus("Ready")
	a.watchStopKey()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.loop.Run(ctx, tick)

	a.start = time.Now()
	if cfg.Debug || a.hasTraceObserver() {
		debug.RegisterState(a.stateSnapshot)
	}
	if cfg.AutoStart {
		a.logger.Info("auto-start requested", "window", c.Capture.Window())
		time.AfterFunc(500*time.Millisecond, func() { a.loop.Do(a.toggleCapture) })
	}
	return a.wails.Run()
}

// tick runs the presenters and sends the state; it runs on the loop.
func (a *app) tick() {
	c := a.container
	c.Loop.Tick()
	x, y, locked := c.FSM.TargetCoordinates()
	var p desktop.Point
	if sel := c.Selection.ActiveRect(); locked && sel != nil {
		p = desktop.Point{X: x - sel.Min.X, Y: y - sel.Min.Y}
	}
	c.View.SetTarget(p, locked && c.Capture.Enabled())
	c.View.Flush()
}

// shutdown stops recording and capture before the app exits.
func (a *app) shutdown() {
	a.loop.Do(func() {
		if a.overlay != nil {
			a.overlay.Close()
		}
		a.container.RecordPresenter.Stop()
		a.container.Engine.Stop()
	})
	a.container.FSM.Close()
}

func (a *app) setWindow(title string) {
	a.container.Capture.SetWindow(title)
	a.container.View.SetWindow(title)
}

// toggleCapture delegates to the capture presenter after checking that a start is safe.
func (a *app) toggleCapture() {
	c := a.container
	if !c.Capture.Enabled() {
		if problem := c.startProblem(); problem != "" {
			a.logger.Error("start refused: " + problem)
			c.View.SetStatus("Cannot start: " + problem)
			return
		}
	}
	c.View.SetStatus("Ready")
	c.CapturePresenter.Toggle()
}

// openSelection shows the overlay that sets the capture area, over the
// current selection or centred on the configured screen.
func (a *app) openSelection() {
	if a.overlay != nil {
		a.overlay.Focus()
		return
	}
	screen, ok := action.ScreenBounds(a.container.Config.Screen)
	if !ok {
		screen = image.Rect(0, 0, 1920, 1080)
	}
	r := a.container.Selection.Start(screen)
	w := a.wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "selection", Title: "Selection", URL: "/selection.html",
		Frameless: true, AlwaysOnTop: true, BackgroundType: application.BackgroundTypeTransparent,
		Windows: application.WindowsWindow{HiddenOnTaskbar: true}, Hidden: true,
	})
	bounds := application.Rect{X: r.Min.X, Y: r.Min.Y, Width: r.Dx(), Height: r.Dy()}
	w.SetPhysicalBounds(bounds)
	w.Show()
	w.SetPhysicalBounds(bounds) // the first Show sizes a window to its options
	a.logger.Debug("selection overlay opened", "want", bounds, "got", w.PhysicalBounds())
	w.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		go a.loop.Do(func() { a.forgetOverlay(w) })
	})
	a.overlay = w
	a.container.View.SetSelection(desktop.RectOf(a.container.Selection.ActiveRect()), true)
}

// confirmSelection selects the overlay's area and closes it.
func (a *app) confirmSelection() {
	if a.overlay == nil {
		return
	}
	b := a.overlay.PhysicalBounds()
	a.container.Selection.Set(image.Rect(b.X, b.Y, b.X+b.Width, b.Y+b.Height))
	a.closeSelection()
}

func (a *app) closeSelection() {
	if w := a.overlay; w != nil {
		a.forgetOverlay(w)
		w.Close()
	}
}

// forgetOverlay drops w, unless a newer overlay replaced it.
func (a *app) forgetOverlay(w *application.WebviewWindow) {
	if a.overlay != w {
		return
	}
	a.overlay = nil
	a.container.View.SetSelection(desktop.RectOf(a.container.Selection.ActiveRect()), false)
}

// watchStopKey registers the global stop hotkey, which also ends a recording.
// It goes straight to the FSM so it still works if the window stalls; the
// capture presenter then stops capture.
func (a *app) watchStopKey() {
	c := a.container
	key := c.Config.StopKey
	vk, ok := action.ParseVK(key)
	if !ok {
		a.logger.Error("unknown stop key", "key", key)
		c.View.SetStatus("Stop hotkey unavailable: unknown key " + key)
		return
	}
	stop := func() {
		c.FSM.EventHalt()
		c.RecordPresenter.RequestStop()
	}
	if err := action.WatchHotkey(vk, stop); err != nil {
		a.logger.Error("stop hotkey unavailable", "key", key, "error", err)
		c.View.SetStatus("Stop hotkey unavailable: " + key + " is in use")
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
