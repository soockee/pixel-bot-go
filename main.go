package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	_ "net/http/pprof" // register pprof handlers when debug enabled
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/soocke/pixel-bot-go/app"
	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/internal/cursormotion"
	"github.com/soocke/pixel-bot-go/internal/inputtiming"
	"github.com/soocke/pixel-bot-go/internal/playtrace"
	"github.com/soocke/pixel-bot-go/internal/recorder"
	"github.com/soocke/pixel-bot-go/internal/traceobserver"
	"github.com/soocke/pixel-bot-go/internal/update"
)

func main() {
	attachConsole()
	if os.Getenv(update.FishChildEnv) != "1" {
		code, err := launchFishProcess()
		if err == nil {
			os.Exit(code)
		}
		fmt.Fprintln(os.Stderr, "fish-named launch unavailable; starting normally:", err)
	}

	cfg, loadErr := config.Load(config.FilePath)
	if loadErr != nil {
		cfg = config.DefaultConfig()
	}
	dryRun := flag.Bool("dry-run", false, "log input actions instead of injecting them (also PIXELBOT_DRY_RUN=1)")
	traceOut := flag.String("trace-out", "", "new JSONL file for dry-run casts, bites, and reels")
	traceSession := flag.String("trace-session", "", "label shared with other traces from this playtest")
	tracePath := flag.String("trace-model", "", "frozen responsebench model for dry-run alerts in /debug/state")
	timingPath := flag.String("timing-model", "", "frozen human timing model")
	timingSeed := flag.Uint64("timing-seed", 1, "input profile seed (random by default for live profiles)")
	liveTiming := flag.Bool("live-timing", false, "use the timing model's hold and cooldown for real input")
	cursorPath := flag.String("cursor-model", "", "frozen human cursor profile")
	liveCursor := flag.Bool("live-cursor", false, "use the cursor profile for real pointer movement")
	record := flag.String("record", "", "record the -window game into this new directory until Ctrl+C; sends no input")
	window := flag.String("window", "", "with -record: the window to record, its picture plus your mouse and key input and its sound")
	recordKeys := flag.Bool("record-keys", false, "with -record: also record keyboard input")
	noKeys := flag.Bool("no-keys", false, "with -record: omit keyboard input")
	listWindows := flag.Bool("list-windows", false, "print visible window titles and exit")
	autoWindow := flag.String("auto-window", "", "preselect the target window title (headless runs)")
	autoStart := flag.Bool("auto-start", false, "start capture automatically after launch (needs -auto-window)")
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
	if *record != "" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		err := recordSession(ctx, recorder.Session{Dir: *record, Window: *window, Label: *traceSession, Keys: *recordKeys && !*noKeys, Status: os.Stderr})
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
	if (*liveTiming && (cfg.DryRun || *timingPath == "")) || (!cfg.DryRun && *timingPath != "" && !*liveTiming) {
		fmt.Fprintln(os.Stderr, "use -timing-model with -dry-run or explicitly enable real timing with -live-timing")
		os.Exit(1)
	}
	if (*liveCursor && (cfg.DryRun || *cursorPath == "")) || (!cfg.DryRun && *cursorPath != "" && !*liveCursor) {
		fmt.Fprintln(os.Stderr, "use -cursor-model with -dry-run or explicitly enable real pointer movement with -live-cursor")
		os.Exit(1)
	}
	if *liveTiming || *liveCursor {
		explicitSeed := false
		flag.Visit(func(f *flag.Flag) { explicitSeed = explicitSeed || f.Name == "timing-seed" })
		if !explicitSeed {
			*timingSeed = rand.Uint64()
		}
	}
	var cursor cursormotion.Planner
	cursorProfile := "default" // cursormotion.DefaultPath
	if *cursorPath != "" {
		model, err := cursormotion.Load(*cursorPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "cursor model:", err)
			os.Exit(1)
		}
		cursor, cursorProfile = model.Planner(*timingSeed), "fitted"
	}
	var timing func() inputtiming.Sample
	var calibration *inputtiming.Calibration
	profile := "fixed"
	if *timingPath != "" {
		model, err := inputtiming.Load(*timingPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "timing model:", err)
			os.Exit(1)
		}
		timing = model.Sampler(*timingSeed)
		calibration = model.Calibration
		profile = "fitted"
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
	if timing != nil {
		logger.Info("timing profile loaded", "dry_run", cfg.DryRun, "seed", *timingSeed)
	}
	if cursor != nil {
		logger.Info("cursor profile loaded", "dry_run", cfg.DryRun, "seed", *timingSeed)
	}
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
	trace := app.TraceOptions{Model: traceModel, Start: time.Now(), Timing: timing, Calibration: calibration, Cursor: cursor}
	if *traceOut != "" {
		utc := trace.Start.UTC()
		var err error
		trace.Output, err = playtrace.Create(*traceOut, playtrace.Header{
			Source: playtrace.AppDryRun, Clock: playtrace.Monotonic, SessionID: *traceSession, StartedUTC: &utc,
			TimingProfile: profile, TimingSeed: timingSeed, CursorProfile: cursorProfile,
		})
		if err != nil {
			logger.Error("create dry-run trace", "error", err)
			os.Exit(1)
		}
	}
	runErr := app.NewApp(fishWindowTitle(), 1180, 760, cfg, logger, trace).Run()
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

func fishWindowTitle() string {
	if name := os.Getenv(update.FishNameEnv); name != "" {
		return name
	}
	return "Go Fish"
}

// recordSession records the game window's client area, fixed where it is when
// the recording starts. It never touches the input path.
func recordSession(ctx context.Context, session recorder.Session) error {
	if session.Window == "" {
		return errors.New("-record needs -window, the game window to record")
	}
	fmt.Fprintln(session.Status, "Recording", session.Window, "to", session.Dir, "- press Ctrl+C to stop")
	return session.Run(ctx)
}
