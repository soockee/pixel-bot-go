# Pixel Bot (Go)

This Go application captures the screen, runs a multi-scale template matching algorithm to locate a target object, and optionally moves the mouse to the detected coordinates. A lightweight Tk GUI shows elapsed time, detection status, and a live scaled preview.

> Detailed documentation of the object search algorithm is in `docs/DETECTION.md`.

## Quickstart

1. Install Go (https://go.dev/dl/) (Go 1.20+ recommended).
2. From PowerShell in the project directory:
3. Make sure to run as administrator to allow mouse control within games.

```powershell
go mod tidy
go build -o pixel-bot.exe
./pixel-bot.exe
```

## Core Features
- Screen capture (Windows) feeding frames to a detection loop.
- Multi-scale, masked normalized cross-correlation (NCC) with optional RGB channel matching.
- Adjustable scale range (`MinScale`, `MaxScale`, `ScaleStep`) or explicit scale list.
- Early-stop on high-confidence score to reduce latency.
- Simple GUI for status + preview.

See `docs/DETECTION.md` for algorithmic details, configuration examples, and future enhancement ideas.

## Safety & Disclaimer
Using automation in games may violate their terms of service. This project is for educational experimentation; use responsibly.

