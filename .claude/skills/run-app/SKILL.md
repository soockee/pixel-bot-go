---
name: run-app
description: Build and launch Pixel Bot together with the fake game window, drive a fishing cycle without sending input, and read the logs and /debug/state to judge behavior. Use when asked to run the app, verify a change end to end, or inspect runtime behavior.
---

# Run Pixel Bot against the fake game

Pixel Bot is a Windows Tk app. Never run it against the real game from here. Use `cmd/fakegame` and `-dry-run` so no input is injected and Administrator is not needed.

## 1. Cheap checks first

```bash
gofmt -l . && go vet ./... && go test ./...
```

If these fail, stop; the end-to-end run adds nothing.

## 2. Build and configure (everything on screen 2)

The user plays the real game on the main screen. Every window of a test run must open on screen 2 (`-screen 2`), never on screen 1. List the monitors first:

```bash
./pixel-bot-go.exe -list-screens      # e.g. [{"screen":2,"x":-1920,"y":148,"w":1920,"h":1080,...}]
```

The app writes `pixel_bot_logs.json` and reads `pixle_bot_config.json` (misspelled on purpose) from its working directory. Run from a scratch directory, not the repo. Selection coordinates are absolute and can be negative (screen 2 may sit left of the main screen). Place the fake game at `-pos 1350,50` (right side of screen 2, clear of the bot window) and select the area around it: screen origin plus (1330, 30), size 560x420.

```bash
RUN="$SCRATCH/pixelbot-run"; mkdir -p "$RUN"
go build -o "$RUN/pixel-bot-go.exe" . && go build -o "$RUN/fakegame.exe" ./cmd/fakegame
cd "$RUN"
# OX, OY = x and y of screen 2 from -list-screens
cat > pixle_bot_config.json <<EOF2
{"debug": true, "screen": 2, "selection_x": $((OX+1330)), "selection_y": $((OY+30)), "selection_w": 560, "selection_h": 420,
 "cooldown_seconds": 3, "threshold": 0.80, "stop_on_score": 0.80}
EOF2
```

`debug: true` enables `/debug/state` on `localhost:6060` and detection score logs. `-auto-start` is refused if the selection lies outside the configured screen, so a mistake cannot capture or click on the main screen. Dry-run is still mandatory for tests.

## 3. Launch headless (no clicking, no Administrator)

Start the bot first, then the fake game so it becomes the foreground window the focus watcher waits for (it briefly takes focus from the game on screen 1; tell the user before running). Run from `$RUN`.

```bash
(./pixel-bot-go.exe -dry-run -screen 2 -auto-window "Fake Game" -auto-start > bot.out 2>&1 &)
sleep 2; (./fakegame.exe -screen 2 -pos 1350,50 > fakegame.out 2>&1 &)
```

`-auto-window` preselects the target window title, `-auto-start` toggles capture 0.5 s after launch, `-dry-run` logs input instead of sending it. The default fake scenario loops every 9.6 s, so wait 25 to 30 s for two or three cycles.

## 4. Stop

```bash
taskkill //IM pixel-bot-go.exe //F; taskkill //IM fakegame.exe //F
```

## 5. Read the signals

- State now: `curl -s localhost:6060/debug/state` (fields: `fsm_state`, `capture_running`, `selected_window`, `target`, `uptime_seconds`).
- History: `pixel_bot_logs.json`, one JSON object per line. Useful messages: `fishing state transition` (`from`, `to`), `found blobber`, `reel action executed`, `dry-run action` (`action`, `x`, `y`).
- Ground truth: `fakegame.out` has one JSON line per phase change (`empty` / `bobber` / `splash`) with timestamps.

## 6. What a healthy cycle looks like

Per default scenario loop (9.6 s): after the bobber appears the bot reaches `monitoring`; within about a second of `splash` it logs a transition to `reeling` then `cooldown` and `dry-run action` entries `move_cursor` and `click_right`. When the bobber is gone it recasts (`press_key`) and returns to `searching`. Compare bot timestamps against `fakegame.out` and report latency and any missed or extra bites.

## Known issues

- Default `threshold: 0.73` can match the desktop wallpaper (scores up to about 0.77 seen), which puts the FSM into `monitoring` at a wrong location. Use 0.80 for fake-game runs.

To merge logs with the fake game's UTC timestamps, note that `pixel_bot_logs.json` uses local time.

## Limits

The bot's real input path (`domain/action`) is not exercised in dry-run, and the fake game cannot validate behavior against the real client's rendering.
