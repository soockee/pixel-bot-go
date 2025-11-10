package main

import (
	"log/slog"
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
	appInstance := app.NewApp("Pixel Bot", 800, 600, cfg, logger)
	if err := appInstance.Run(); err != nil {
		logger.Error("app run failed", "error", err)
		os.Exit(1)
	}
}
