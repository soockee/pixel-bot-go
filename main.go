package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // register pprof handlers when debug enabled
	"os"

	"github.com/soocke/pixel-bot-go/app"
	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/action"
)

func main() {
	cfg, loadErr := config.Load("pixle_bot_config.json")
	if loadErr != nil {
		cfg = config.DefaultConfig()
	}
	dryRun := flag.Bool("dry-run", false, "log input actions instead of injecting them (also PIXELBOT_DRY_RUN=1)")
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
	if *screen > 0 {
		cfg.Screen = *screen
	}
	cfg.AutoWindow, cfg.AutoStart = *autoWindow, *autoStart
	cfg.DryRun = *dryRun || os.Getenv("PIXELBOT_DRY_RUN") == "1"
	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}
	logger := NewLogger(level)
	if loadErr != nil {
		logger.Warn("config load failed; using defaults", "error", loadErr)
	}

	// Conditional pprof server for profiling memory / CPU when debug is enabled.
	// Accessible at http://localhost:6060/debug/pprof/
	if cfg.Debug {
		const pprofAddr = "localhost:6060"
		go func() {
			logger.Info("starting pprof server", "addr", pprofAddr)
			if err := http.ListenAndServe(pprofAddr, nil); err != nil {
				logger.Warn("pprof server stopped", "error", err)
			}
		}()
	}
	appInstance := app.NewApp("Pixel Bot", 800, 600, cfg, logger)
	if err := appInstance.Run(); err != nil {
		logger.Error("app run failed", "error", err)
		os.Exit(1)
	}
}
