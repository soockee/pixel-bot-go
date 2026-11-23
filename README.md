# Go Fish

Go Fish fishes for you in World of Warcraft: Forever. A small addon marks your bobber, Go Fish watches it for the bite, and an ESP32 board, paired as a Bluetooth keyboard and mouse, casts and reels in.

https://github.com/user-attachments/assets/b5e026ad-c879-4a48-ba24-902346012309

[Download for Windows](https://github.com/soockee/pixel-bot-go/releases/latest) · [Player guide](https://soockee.github.io/pixel-bot-go/) · [Buy me a coffee](https://buymeacoffee.com/socke)

## What you need

- A Windows PC with World of Warcraft: Forever and a character who can fish
- An ESP32 board (the original ESP32, such as an ESP32-WROOM-32 development board) with the [Go Fish firmware](firmware/esp32_ble_probe), paired in Windows Bluetooth settings and plugged in by USB. Every key and click Go Fish sends goes through this board

## Get started

1. Download `pixel-bot-go-windows-amd64.exe` from the [latest release](https://github.com/soockee/pixel-bot-go/releases/latest). `CHECKSUMS.txt` next to it lists its SHA-256.
2. Start the game, log in, then start Go Fish. **Set up fishing** walks you through four steps:
   - **Game window**: found by itself
   - **ESP32 board**: click **Find and test the board**. Go Fish checks that the board answers over USB and that its test key reaches Windows over Bluetooth
   - **Game keys**: F9 casts. F10 (Interact) and F12 (Logout) are asked for only when your settings use them. WoW leaves these keys free; keep them or press other spare keys, then click **Use these keys**
   - **Bobber Marker addon**: click **Install addon**, then type `/reload` in the game
3. Turn on **Auto Loot** in the game, stand at the water, click **Start fishing** in Go Fish, and click into the game.

The [first catch walkthrough](https://soockee.github.io/pixel-bot-go/tutorials/first-catch/) covers each step in more detail.

## How it works

- **Seeing the bobber.** Bobber Marker draws pink brackets around your own bobber, and only while you fish. Go Fish finds them in the game window and from then on watches only that box.
- **The bite.** A bite is the splash at your bobber followed by the game's bite sound, so other players' bites and spells don't fool it. Keep the game's sound effects on: without them, Go Fish needs three casts without a reel before it reels on the splash alone.
- **Reeling in.** The board moves the pointer onto the bobber and right-clicks, or presses your Interact key if you pick that reel method. Then Go Fish pauses and casts again.
- **Stopping.** A run ends after the session limit (60 minutes by default) or five casts in a row without a bite. The Fishing view then says why it stopped, with a link to what to do.

While it fishes, leave the mouse alone and keep the game in front. Moving the mouse stops the run, so Go Fish never fights you for it.

## In the app

- **Fishing**: what Go Fish sees, the step it is on (Focus, Search, Watch, Reel, Pause, Cast), your catches this run and how each reel ended: caught, got away or not hooked
- **Overlay**: a small pill over the game that shows how the run goes. It takes no clicks, and you can move it out of your way
- **Between catches**, all optional: move the mouse round the game while watching the bobber, change spot every 15 to 25 catches, and take logged-out breaks on a schedule
- **Updates**: Go Fish checks GitHub when it starts and every hour, verifies the download against the release's checksum and replaces itself. **Settings → Addon** installs and updates Bobber Marker
- **Problems**: **Report a problem** opens a GitHub issue in your browser with the version and recent warnings filled in; nothing is sent until you submit it. See [what Go Fish sends](https://soockee.github.io/pixel-bot-go/explanation/data-sharing/)

## Develop

Go 1.26 and Node 24 on Windows. In PowerShell:

```powershell
go run ./cmd/fakegame    # a simulated fishing scene with scripted bites
./run.ps1 -dry-run       # builds the frontend and the app, then starts it
go test ./...
```

Pick **Fake Game** as the game window and click **Start fishing**. A dry run logs the keys, pointer moves and clicks it would send to `pixel_bot_logs.json` instead of sending them, so it needs no board. [AGENTS.md](AGENTS.md) maps the code and its conventions, and [`docs/dev`](docs/dev) holds the technical docs.

## License

MIT, see [LICENSE](LICENSE).
