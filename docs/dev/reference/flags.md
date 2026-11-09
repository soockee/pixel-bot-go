---
title: "Command-line flags"
summary: "Flags and environment variables for scripted and headless runs."
order: 3
---

| Flag / variable                 | Effect                                                                                      |
| ------------------------------- | ------------------------------------------------------------------------------------------- |
| `-dry-run` / `PIXELBOT_DRY_RUN=1` | Log key presses, cursor moves and clicks as `dry-run action` instead of sending them; no Administrator needed |
| `-screen N` / config `screen`   | Open the window on monitor N (1 = primary, others left to right) and refuse a run whose game window is elsewhere; 0 = default placement |
| `-list-screens`                 | Print the monitors as JSON (`screen`, `x`, `y`, `w`, `h`, `primary`) and exit               |
| `-record <dir>`                 | With `-window`: record that window's client area into a new directory until Ctrl+C (whole frames every 10 s, the bobber's square at about 10 fps, see Detection datasets); opens no app window and sends no input |
| `-window "<title>"`             | With `-record`: the window to record, plus your mouse and key input in it to `input.jsonl` and the sound of its process to `audio.wav` |
| `-record-keys`                  | With `-record`: also capture keyboard input locally |
| `-list-windows`                 | Print visible window titles and exit |
| `-auto-window "<title>"`        | Preselect the target window title (must match the window title exactly, case-insensitive)   |
| `-auto-start`                   | Start capture 0.5 s after launch; refused unless the `-auto-window` window is open |

With `"debug": true` in the config, `http://localhost:6060/debug/state` returns the live FSM state as JSON, the window's WebView2 DevTools listen on `localhost:9222`, and `detection.search` entries (found, score, scale, position; misses sampled 1 in 10) are written to the log. `go run ./cmd/fakegame` opens a scripted simulator scene for local testing (`-scenario file.json`, `-pos x,y`, `-title`).
