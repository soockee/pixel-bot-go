# WoW Fishing Bot (Go)

This small Go project captures a screen region, attempts simple bobber detection by hue, and simulates a mouse click when a bite is detected.

Important notes:
- This is written for Windows. It uses `github.com/kbinani/screenshot` for capturing and `github.com/go-vgo/robotgo` for input simulation.
- Build and run on Windows with Go installed (1.20+).

Quickstart:

1. Install Go (https://go.dev/dl/)
2. From PowerShell in the `go-bot` directory:

```powershell
go mod tidy
go build -o wowzer-bot.exe
.
```

3. Run the binary and adjust `config.json` to set the capture region or thresholds.

Limitations and safety:
- This project is a starting point and uses naive detection. For production you should implement robust image processing.
- Using automation with games may violate their terms of service. Use responsibly.
