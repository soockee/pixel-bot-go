package main

import (
	"log/slog"

	"github.com/soocke/pixel-bot-go/app"
	"github.com/soocke/pixel-bot-go/config"
)

func main() {
	// Base config from defaults
	cfg := config.DefaultConfig()

	// Set up logger
	logger := NewLogger(slog.LevelInfo)

	application := app.NewApp("Pixel Bot", 800, 600, cfg, logger)
	application.Start()
}
