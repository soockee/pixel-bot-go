package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // register pprof handlers when debug enabled
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/soocke/pixel-bot-go/app"
	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/domain/capture"
	"github.com/soocke/pixel-bot-go/internal/playtrace"
	"github.com/soocke/pixel-bot-go/internal/recorder"
	"github.com/soocke/pixel-bot-go/internal/traceobserver"
)

func main() {
	cfg, loadErr := config.Load(config.FilePath)
	if loadErr != nil {
		cfg = config.DefaultConfig()
	}
	dryRun := flag.Bool("dry-run", false, "log input actions instead of injecting them (also PIXELBOT_DRY_RUN=1)")
	traceOut := flag.String("trace-out", "", "new JSONL file for dry-run casts, bites, and reels")
	traceSession := flag.String("trace-session", "", "label shared with other traces from this playtest")
	tracePath := flag.String("trace-model", "", "frozen responsebench model for dry-run alerts in /debug/state")
	record := flag.String("record", "", "record the saved selection into this new directory until Ctrl+C; sends no input")
	window := flag.String("window", "", "with -record: also record your mouse and key input in the window with this exact title")
	noKeys := flag.Bool("no-keys", false, "with -record: record only mouse input")
	listen := flag.String("listen", recorder.DefaultListen, "with -record or -view: loopback dashboard address; empty disables it")
	view := flag.String("view", "", "open the dashboard for a saved recording directory or trace file")
	listWindows := flag.Bool("list-windows", false, "print visible window titles and exit")
	autoWindow := flag.String("auto-window", "", "preselect the target window title (headless runs)")
	autoStart := flag.Bool("auto-start", false, "start capture automatically after launch (needs a saved selection and -auto-window)")
	screen := flag.Int("screen", 0, "monitor to open windows on: 1 = primary, 2 = next from the left (0 = default; also config \"screen\")")
	listScreens := flag.Bool("list-screens", false, "print the monitors as JSON and exit")
	flag.Parse()
	if *listScreens {
		screens, err := action.ListScreens()
		if err != nil {
			fmt.Fprintln(os.Stderr, "list-screens:", err)
			os.Exit(1)
		}
		_ = json.NewEncoder(os.Stdout).Encode(screens)
		return
	}
	if *listWindows {
		titles, err := action.ListWindows()
		if err != nil {
			fmt.Fprintln(os.Stderr, "list-windows:", err)
			os.Exit(1)
		}
		fmt.Println(strings.Join(titles, "\n"))
		return
	}
	if *view != "" || *record != "" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		var err error
		if *view != "" {
			err = recorder.View(ctx, *view, *listen, os.Stderr)
		} else {
			err = recordSession(ctx, cfg, recorder.Session{Dir: *record, Window: *window, Listen: *listen, Label: *traceSession, Keys: !*noKeys, Status: os.Stderr})
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "recorder:", err)
			os.Exit(1)
		}
		return
	}
	if *screen > 0 {
		cfg.Screen = *screen
	}
	cfg.AutoWindow, cfg.AutoStart = *autoWindow, *autoStart
	cfg.DryRun = *dryRun || os.Getenv("PIXELBOT_DRY_RUN") == "1"
	if !cfg.DryRun && (*tracePath != "" || *traceOut != "") {
		fmt.Fprintln(os.Stderr, "-trace-model and -trace-out require -dry-run")
		os.Exit(1)
	}
	var traceModel *traceobserver.Model
	if *tracePath != "" {
		var err error
		traceModel, err = traceobserver.LoadModel(*tracePath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "trace model:", err)
			os.Exit(1)
		}
	}
	debugEnabled := cfg.Debug || traceModel != nil
	level := slog.LevelInfo
	if debugEnabled {
		level = slog.LevelDebug
	}
	logger := NewLogger(level)
	slog.SetDefault(logger)
	if loadErr != nil {
		logger.Warn("config load failed; using defaults", "error", loadErr)
	}

	// Conditional pprof server for profiling memory / CPU when debug is enabled.
	// Accessible at http://localhost:6060/debug/pprof/
	if debugEnabled {
		const pprofAddr = "localhost:6060"
		go func() {
			logger.Info("starting pprof server", "addr", pprofAddr)
			if err := http.ListenAndServe(pprofAddr, nil); err != nil {
				logger.Warn("pprof server stopped", "error", err)
			}
		}()
	}
	trace := app.TraceOptions{Model: traceModel, Start: time.Now()}
	if *traceOut != "" {
		utc := trace.Start.UTC()
		var err error
		trace.Output, err = playtrace.Create(*traceOut, playtrace.Header{
			Source: playtrace.AppDryRun, Clock: playtrace.Monotonic, SessionID: *traceSession, StartedUTC: &utc,
		})
		if err != nil {
			logger.Error("create dry-run trace", "error", err)
			os.Exit(1)
		}
	}
	appInstance, err := app.NewApp("Visual Response Lab", 800, 600, cfg, logger, trace)
	if err != nil {
		logger.Error("app setup failed", "error", err)
		os.Exit(1)
	}
	runErr := appInstance.Run()
	if trace.Output != nil {
		status := "complete"
		if runErr != nil {
			status = "incomplete"
		}
		if err := trace.Output.Close(playtrace.End{ElapsedUS: time.Since(trace.Start).Microseconds(), Status: status}); err != nil {
			logger.Error("finalize dry-run trace", "error", err)
			os.Exit(1)
		}
	}
	if err := runErr; err != nil {
		logger.Error("app run failed", "error", err)
		os.Exit(1)
	}
}

// recordSession records the saved selection. It never touches the input path.
func recordSession(ctx context.Context, cfg *config.Config, session recorder.Session) error {
	if cfg.SelectionW <= 0 || cfg.SelectionH <= 0 {
		return errors.New("no saved selection (use Selection in the app, or set selection_x/y/w/h in the config)")
	}
	region := image.Rect(cfg.SelectionX, cfg.SelectionY, cfg.SelectionX+cfg.SelectionW, cfg.SelectionY+cfg.SelectionH)
	session.Frames = capture.Source{Region: func() *image.Rectangle { return &region }}
	fmt.Fprintln(session.Status, "Recording", region, "to", session.Dir, "- press Ctrl+C to stop")
	return session.Run(ctx)
}
