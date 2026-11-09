package app

import (
	"context"
	"image"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/soocke/pixel-bot-go/addon"
	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/debug"
	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/domain/fishing"
	"github.com/soocke/pixel-bot-go/frontend"
	"github.com/soocke/pixel-bot-go/internal/esp32"
	"github.com/soocke/pixel-bot-go/internal/update"
	"github.com/soocke/pixel-bot-go/ui/desktop"
)

const (
	tick         = 33 * time.Millisecond
	devtoolsPort = "9222" // WebView2 DevTools on localhost, debug mode only
)

func init() { application.RegisterEvent[desktop.State](desktop.StateEvent) }

type app struct {
	container       *AppContainer
	logger          *slog.Logger
	start           time.Time
	loop            *desktop.Loop
	wails           *application.App
	main            *application.WebviewWindow
	overlay         *application.WebviewWindow
	overlayPlaced   overlayPlacement // what the overlay window was last set to; used on the loop
	overlayMove     bool             // the person is placing the overlay; used on the loop
	overlayReady    bool             // passThrough set the overlay's styles; used on the main thread
	pillMu          sync.Mutex       // guards pill, which the main thread writes and the loop reads
	pill            image.Rectangle  // the overlay window while it is moved, wherever the person left it
	updates         *update.Client
	release         update.Release // the latest release found, zero before the first check; used on the loop
	restart         string         // the executable to start once the app quit, after an update
	schedule        *playSchedule
	recoveryAllowed bool
	scheduleStop    atomic.Bool
	runSeconds      int  // how long the current run has fished; used on the loop
	wasRunning      bool // a run went on at the last tick; used on the loop
}

// NewApp builds the components and the window.
func NewApp(title string, width, height int, cfg *config.Config, logger *slog.Logger, trace TraceOptions) *app {
	a := &app{logger: logger}
	a.container = BuildContainer(cfg, logger, trace, func(s desktop.State) { a.wails.Event.Emit(desktop.StateEvent, s) })
	c := a.container
	a.updates = update.New()
	a.loop = desktop.NewLoop(a.tick)
	service := desktop.NewService(a.loop, c.View, desktop.Commands{
		ListWindows: func() []string {
			titles, err := action.ListWindows()
			if err != nil {
				logger.Error("list windows failed", "error", err)
			}
			return titles
		},
		ListPorts: func() []string {
			ports, err := esp32.Ports()
			if err != nil {
				logger.Error("list COM ports failed", "error", err)
			}
			return ports
		},
		SetWindow:     a.setWindow,
		ToggleCapture: a.toggleCapture,
		ToggleRecord: func() {
			// A recording may run beside the bot: bot.jsonl then holds its decisions.
			c.RecordPresenter.Toggle()
		},
		Running:       func() bool { return c.Capture.Enabled() || a.schedule != nil },
		Exit:          func() { go a.wails.Quit() }, // Quit waits for the loop, which runs this
		Update:        a.installUpdate,
		CheckUpdate:   a.findUpdate,
		ReleaseNotes:  a.releaseNotes,
		ReportProblem: a.reportProblem,
		OpenURL:       func(url string) { a.wails.Browser.OpenURL(url) },
		CheckAddon:    a.checkAddon,
		InstallAddon:  a.installAddon,
		MoveOverlay:   a.setOverlayMove,
		SetTheme:      a.setTheme,
	}, c.Recordings, cfg, config.FilePath, logger)

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
	a.overlay = a.newOverlay()
	a.main.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) { go a.wails.Quit() })
	// The native window exists only once the app runs, so it is placed then.
	a.wails.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		if b, ok := action.ScreenBounds(cfg.Screen); ok {
			p := a.main.PhysicalBounds()
			a.main.SetPhysicalBounds(application.Rect{X: b.Min.X + 100, Y: b.Min.Y + 100, Width: p.Width, Height: p.Height})
		} else if cfg.Screen > 0 {
			logger.Warn("configured screen not found; using default placement", "screen", cfg.Screen)
		}
		a.setTheme(cfg.Theme)
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
	c.View.SetInfo(cfg.DryRun, cfg.Debug)
	c.View.SetTheme(cfg.Theme)
	c.View.SetOverlay(cfg.OverlayX, cfg.OverlayY, false)
	c.View.SetAddonVersion(addon.Version())
	a.checkAddon()
	c.View.SetStatus("Ready")
	c.View.SetVersion(update.Version)
	a.checkUpdate()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.loop.Run(ctx, tick)

	a.start = time.Now()
	if cfg.Debug || a.hasTraceObserver() {
		debug.RegisterState(a.stateSnapshot)
	}
	if cfg.AutoStart {
		a.logger.Info("auto-start requested", "window", c.Capture.Window())
		a.autoStart(60)
	}
	err := a.wails.Run()
	if a.restart != "" {
		if rerr := update.Restart(a.restart); rerr != nil {
			a.logger.Error("restart after update failed", "error", rerr)
		}
	}
	return err
}

// autoStart starts capture in half a second, trying again while the game
// window is not open yet, at most tries times.
func (a *app) autoStart(tries int) {
	time.AfterFunc(500*time.Millisecond, func() {
		a.loop.Do(func() {
			if a.schedule != nil || a.container.Capture.Enabled() {
				return
			}
			if tries > 1 && (a.container.bindGameWindow() != nil || a.container.startProblem() != "") {
				a.autoStart(tries - 1)
				return
			}
			a.toggleCapture()
		})
	})
}

// tick runs the presenters and sends the state; it runs on the loop.
func (a *app) tick() {
	c := a.container
	c.Loop.Tick()
	a.tickSchedule(time.Now())
	if a.overlayMove && c.Capture.Enabled() {
		_ = a.setOverlayMove(false) // a run owns the overlay; keep the place the person chose
	}
	a.followGame()
	run := c.FSM.Stats()
	a.countCatches(run, c.View.State())
	c.View.SetRun(run.Casts, run.Reels, run.ReelAttempts, run.ReelInputs, run.FailedInRow, c.Config.MaxFailedCasts, c.Config.MaxSessionMinutes)
	c.View.SetOutcomes(run.Caught, run.GotAway, run.NotHooked)
	c.View.Flush()
}

// shutdown stops recording and capture before the app exits.
func (a *app) shutdown() {
	a.scheduleStop.Store(true)
	a.container.FSM.Close()
	a.loop.Do(func() {
		a.schedule = nil
		a.container.RecordPresenter.Stop()
		a.container.Engine.Stop()
	})
}

func (a *app) setWindow(title string) {
	if a.schedule != nil || a.container.Capture.Enabled() ||
		a.container.FSM.Current() != fishing.StateHalt || a.container.View.State().Recording {
		return
	}
	a.container.gameWindow.Store(nil)
	a.container.Capture.SetWindow(title)
	a.container.View.SetWindow(title)
}

// toggleCapture delegates to the capture presenter after checking that a start is safe.
func (a *app) toggleCapture() {
	c := a.container
	if a.schedule != nil {
		c.FSM.TakeRepositionLogout()
		a.stopSchedule("Schedule stopped")
		return
	}
	if !c.Capture.Enabled() {
		if err := c.bindGameWindow(); err != nil {
			c.View.SetStatus("Cannot start: " + err.Error())
			return
		}
		if problem := c.startProblem(); problem != "" {
			a.logger.Error("start refused: " + problem)
			c.View.SetStatus("Cannot start: " + problem)
			return
		}
		if err := fishing.CheckActivityKeys(*c.Config, action.ParseVK); err != nil {
			c.View.SetStatus("Cannot start: " + err.Error())
			return
		}
		if c.Config.Reposition && !c.Config.Schedule {
			if _, err := a.prepareSchedule(); err != nil {
				c.View.SetStatus("Cannot enable repositioning: " + err.Error())
				return
			}
		}
		if c.Config.Schedule {
			if err := a.beginSchedule(); err != nil {
				c.View.SetStatus("Cannot start schedule: " + err.Error())
			}
			return
		}
	}
	a.recoveryAllowed = !c.Capture.Enabled()
	if !a.recoveryAllowed {
		a.scheduleStop.Store(true)
	}
	c.View.SetStatus("Ready")
	c.CapturePresenter.Toggle()
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
