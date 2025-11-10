package main

import (
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
