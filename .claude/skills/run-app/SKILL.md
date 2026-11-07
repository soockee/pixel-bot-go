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

## 2. Build into a scratch run directory

The app writes `pixel_bot_logs.json` and reads `pixle_bot_config.json` (misspelled on purpose) from its working directory. Run from a scratch directory, not the repo.

```bash
RUN="$SCRATCH/pixelbot-run"; mkdir -p "$RUN"
go build -o "$RUN/pixel-bot-go.exe" . && go build -o "$RUN/fakegame.exe" ./cmd/fakegame
cat > "$RUN/pixle_bot_config.json" <<'EOF2'
{"debug": true, "selection_x": 30, "selection_y": 30, "selection_w": 560, "selection_h": 420,
 "cooldown_seconds": 3, "threshold": 0.80, "stop_on_score": 0.80, "stride": 2}
EOF2
```

The selection rectangle (screen pixels) must cover the fake game window placed at `-pos 50,50` (window is 480x320 plus title bar). `stride: 2` is deliberate: at the default stride 6 the 17x10 bobber is missed at some pixel alignments (see Known issues). `debug: true` enables `/debug/state` on `localhost:6060` and detection score logs.

## 3. Launch headless (no clicking, no Administrator)

Start the bot first, then the fake game so it becomes the foreground window the focus watcher waits for. Run from `$RUN`.

```bash
(./pixel-bot-go.exe -dry-run -auto-window "Fake Game" -auto-start > bot.out 2>&1 &)
sleep 2; (./fakegame.exe -pos 50,50 > fakegame.out 2>&1 &)
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

- Default `stride: 6` misses the bobber at some screen alignments even with `refine` on (live grab: best score 0.54 at stride 6, 0.90 at stride 3, 1.00 at stride 1). Look for `detection.search` entries with `found:false` while `fakegame.out` says `bobber`.
- Default `threshold: 0.73` can match the desktop wallpaper (scores up to about 0.77 seen), which puts the FSM into `monitoring` at a wrong location. Use 0.80 for fake-game runs.

To merge logs with the fake game's UTC timestamps, note that `pixel_bot_logs.json` uses local time.

## Limits

The bot's real input path (`domain/action`) is not exercised in dry-run, and the fake game cannot validate behavior against the real client's rendering.
