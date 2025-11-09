package main

import (
	"log/slog"

	"github.com/soocke/pixel-bot-go/app"
	"github.com/soocke/pixel-bot-go/config"
)

func main() {
	// Attempt to load persistent config (including selection rectangle).
	cfg, err := config.Load("pixle_bot_config.json")
	if err != nil {
		// Fallback to defaults on error; logging after logger init.
		cfg = config.DefaultConfig()
	}

	// Set up logger
	var loglevel slog.Level
	if cfg.Debug {
		loglevel = slog.LevelDebug
	} else {
		loglevel = slog.LevelInfo
	}
	logger := NewLogger(loglevel)
	if err != nil {
		logger.Warn("failed to load pixle_bot_config.json; using defaults", "error", err)
	}

	application := app.NewApp("Pixel Bot", 800, 600, cfg, logger)
	application.Start()
}
