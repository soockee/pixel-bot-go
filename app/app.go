package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/debug"
	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/frontend"
	"github.com/soocke/pixel-bot-go/internal/cloud"
	"github.com/soocke/pixel-bot-go/internal/update"
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
	cloud     *cloud.Client  // nil while sign-in is not configured
	session   *cloud.Session // the signed-in account, nil when signed out; used on the loop
	updates   *update.Client
	release   update.Release // the newer release on offer, zero when none; used on the loop
	restart   string         // the executable to start once the app quit, after an update
}

// NewApp builds the components and the window.
func NewApp(title string, width, height int, cfg *config.Config, logger *slog.Logger, trace TraceOptions) *app {
	a := &app{logger: logger}
	a.container = BuildContainer(cfg, logger, trace, func(s desktop.State) { a.wails.Event.Emit(desktop.StateEvent, s) })
	c := a.container
	a.cloud = a.newCloud()
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
		SetWindow:     a.setWindow,
		ToggleCapture: a.toggleCapture,
		ToggleRecord:  c.RecordPresenter.Toggle,
		Running:       c.Capture.Enabled,
		Exit:          func() { go a.wails.Quit() }, // Quit waits for the loop, which runs this
		SignIn:        a.signIn,
		SignOut:       a.signOut,
		Upload:        a.upload,
		Update:        a.installUpdate,
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
	c.View.SetInfo(cfg.DryRun, cfg.Debug, cfg.StopKey, cfg.DarkMode)
	c.View.SetStatus("Ready")
	c.View.SetVersion(update.Version)
	a.watchStopKey()
	a.restoreAccount()
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
			if tries > 1 && a.container.startProblem() != "" {
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
	run := c.FSM.Stats()
	c.View.SetRun(run.Casts, run.Reels, run.FailedInRow, c.Config.MaxFailedCasts, c.Config.MaxSessionMinutes)
	c.View.Flush()
}

// shutdown stops recording and capture before the app exits.
func (a *app) shutdown() {
	a.container.FSM.Close()
	a.loop.Do(func() {
		a.container.RecordPresenter.Stop()
		a.container.Engine.Stop()
	})
}

func (a *app) setWindow(title string) {
	a.container.Capture.SetWindow(title)
	a.container.View.SetWindow(title)
}

// toggleCapture delegates to the capture presenter after checking that a start is safe.
func (a *app) toggleCapture() {
	c := a.container
	if !c.Capture.Enabled() {
		// A dry run sends no input, so tests and agents need no account.
		if !c.Config.DryRun && !a.can(cloud.PermFish) {
			a.logger.Warn("start refused: the account may not fish", "signed_in", a.session != nil)
			c.View.SetStatus("Sign in with an account whose role may fish")
			return
		}
		if problem := c.startProblem(); problem != "" {
			a.logger.Error("start refused: " + problem)
			c.View.SetStatus("Cannot start: " + problem)
			return
		}
	}
	c.View.SetStatus("Ready")
	c.CapturePresenter.Toggle()
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
