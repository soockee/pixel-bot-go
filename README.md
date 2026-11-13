# Go Fish

Go Fish is a Windows app for studying visual event detection and response timing, using fishing in World of Warcraft: Forever as the test scene. A small addon marks your bobber, the app finds the marker in the game window, watches the box for the splash of a bite, and answers with a click. A local simulator plays the same scene with scripted bites, so everything can be tested without the game.

**Documentation:** [soockee.github.io/pixel-bot-go](https://soockee.github.io/pixel-bot-go/)

[![Buy me a coffee](https://img.shields.io/badge/Buy%20me%20a%20coffee-socke-ffdd00?logo=buymeacoffee&logoColor=black)](https://buymeacoffee.com/socke)

## Install

1. Download `pixel-bot-go-windows-amd64.exe` from the [latest release](https://github.com/soockee/pixel-bot-go/releases/latest) (`CHECKSUMS.txt` beside it lists its SHA-256). Connect and pair a [Go Fish ESP32 Probe](https://github.com/soockee/pixel-bot-go/tree/main/firmware/esp32_ble_probe), then start the game and the exe.
2. In **Settings → ESP32 board**, choose **Auto** or the board's COM port. The paired board sends all live game input over Bluetooth.
3. In **Set up fishing** on the Fishing tab, click **Find the board**, confirm the suggested F9 to F12 keys with **Use these keys**, then click **Install addon** to put the Bobber Marker addon into the game, then type `/reload` in the game.
4. Stand at the water and press **Start fishing**. Click **Stop** in Go Fish to end the run.

Go Fish updates itself: it checks GitHub at start and every hour, and **Update to vX.Y.Z** in the top bar or **Settings → Updates** installs a new release after checking its checksum.

## What it does

* Finds the bobber by the magenta brackets the Bobber Marker addon draws, then watches only that box at 30 fps.
* Detects a bite from the splash at your bobber and the bite sound that follows it, and still works with the game's sound effects off.
* Runs a state machine: cast, search, watch, reel, pause, and stops after a session limit or too many casts without a bite.
* Stops when you click **Stop** in Go Fish.
* Dry-run mode logs intended key presses, clicks and cursor moves. It works without an ESP32.
* Records sessions (frames, input and sound) for comparing detectors offline.

## Develop

Go 1.26 and Node 24 on Windows. In PowerShell:

```powershell
go run ./cmd/fakegame          # the simulator, in one terminal
./run.ps1 -dry-run             # builds the frontend and the app, then starts it
go test ./...
```

Pick **Fake Game** as the game window and press **Start fishing**; the log `pixel_bot_logs.json` shows each state change and the clicks a real run would send. [AGENTS.md](AGENTS.md) describes the layout and conventions, and [`docs/dev`](docs/dev) holds the technical docs: detection, config and flags, the benchmark, profiling.

## Documentation

The player guide follows [Diataxis](https://diataxis.fr/):

* [Get started](https://soockee.github.io/pixel-bot-go/tutorials/first-catch/): your first catch.
* [How-to guides](https://soockee.github.io/pixel-bot-go/#how-to): install the addon, change settings, update, fix problems.
* [Reference](https://soockee.github.io/pixel-bot-go/#reference): settings, the Fishing tab, addon commands, files.
* [Understand](https://soockee.github.io/pixel-bot-go/#explanation): how Go Fish works, and [what Go Fish sends](https://soockee.github.io/pixel-bot-go/explanation/data-sharing/).

## Support

Go Fish is free and made in spare time. If it saves you time, you can [buy me a coffee](https://buymeacoffee.com/socke).

## License

MIT, see [LICENSE](LICENSE).
