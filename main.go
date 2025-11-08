package main

import (
	"github.com/soocke/pixel-bot-go/app"
	"github.com/soocke/pixel-bot-go/config"
)

func main() {
	config := &config.Config{
		Debug: true,
	}
	app := app.NewApp("My Application", 800, 600, config)
	app.Start()
}
