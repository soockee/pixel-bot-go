package main

import (
	"flag"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // register pprof handlers when debug enabled
	"os"

	"github.com/soocke/pixel-bot-go/app"
	"github.com/soocke/pixel-bot-go/config"
)

func main() {
	cfg, loadErr := config.Load("pixle_bot_config.json")
	if loadErr != nil {
		cfg = config.DefaultConfig()
	}
	dryRun := flag.Bool("dry-run", false, "log input actions instead of injecting them (also PIXELBOT_DRY_RUN=1)")
	autoWindow := flag.String("auto-window", "", "preselect the target window title (headless runs)")
	autoStart := flag.Bool("auto-start", false, "start capture automatically after launch (needs a saved selection and -auto-window)")
	flag.Parse()
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
