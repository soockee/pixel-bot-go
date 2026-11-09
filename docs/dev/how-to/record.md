---
title: "Record and evaluate sessions"
summary: "Record frames, input and sound from the game, upload them, and compare detectors on them offline."
order: 4
---

A recording saves the screen and your input together, on the same clock, and sends no input. The Recorder tab is a developer tool: it appears with `"debug": true` in `pixle_bot_config.json`.

1. Start the app. On the **Fishing** tab, pick the game window (the list reloads when you open it).
2. Open the **Recorder** tab and click **Record**. The new recording opens and follows your input live.
3. Focus the game and play by hand.
4. Click **Stop recording** in the app.

Each recording gets a new timestamped folder holding `frames/`, `input.jsonl` and the game's sound in `audio.wav`. You still hear the game while it records, and music or voice chat from other programs is not recorded. Set **Recording folder** and **Record keys** on the **Settings** tab (debug mode). The default `datasets/` folder is ignored by git. Start stays disabled while recording, and Record while the bot runs.

For scripts and agents, the same recording runs without a window: `pixel-bot-go.exe -record <dir> -window "<title>"`, stopped with Ctrl+C. It records the window's client area where it is at the start. Add `-record-keys` to capture keyboard input locally. `-list-windows` prints the titles.

The recorder stores client-area cursor positions, relative mouse counts, both wheel axes, and press and release events for left, right, middle, Mouse 4, and Mouse 5. It also records key down and up with virtual-key and scan codes, repeat status, and shortcuts such as Shift+B. Mouse capture requires the pointer over the selected client area. Keyboard capture follows the selected foreground window, including its chat or text fields. Keyboard capture is off unless requested. Keyboard input Windows marks as injected is skipped.

The JSONL keeps the raw button transitions and adds one gesture per completed hold. A hold becomes a drag if it moves at least 6 client pixels or reaches at least 8 raw counts away from its start. Gesture rows include duration, endpoints, total raw travel, and maximum excursions. A right-button camera drag can be detected even with a stationary cursor. Holds cut off by a capture gap are marked interrupted. A final `session_end` row reports the event counts and whether the file finished cleanly. The recorder captures input and key bindings. Bite and cast annotations are separate.

## Upload a recording

Open a finished recording on the **Recorder** tab and click **Upload**. It needs a role with `recordings.upload`. The status bar counts the files; anything already uploaded is skipped, so start it again after a failure. Uploaded files cannot be replaced.

## View a recording

The **Recorder** tab lists the recordings in the recording folder, newest first. Pick one to see its counts, the latest frame, the cursor map (movement density, completed click locations and the recent path), the raw right-button camera motion, the activity of the last minute and a log of gestures and shortcuts. A recording in progress updates live. Dry-run and fake-game traces in a recording folder show their cast and bite counts instead.

## Evaluate detectors

Label a session from your own casts and reels, giving your bindings:

```powershell
go run ./cmd/detecteval -label datasets/zone1-day -cast mouse:x1 -reel mouse:right
```

Check a few casts against the frames, because the bite times are estimates: the file names are milliseconds from the start, and `-reaction` (default 1200) is how long you take to click after a bite. It will not replace an existing `labels.json`, so hand corrections are safe. Do not move the game window while recording, or the bobber boxes will be wrong.

Then pass every dataset to include:

```powershell
go run ./cmd/detecteval datasets/zone1-day datasets/zone1-night
```

This compares every registered technique, one column each. Use `-locator` or `-bite` to compare only some.

## Add and try a technique

1. Implement `detect.Locator` or `detect.BiteDetector`.
2. Add one line for it in `domain/detect/registry.go`.
3. Run `detecteval`. The new technique appears as a column.
4. To use it in the app, set `"debug": true` and pick it on the **Settings** tab, under **Locator** or **Bite detector**.

## Export data for training

```powershell
go run ./cmd/detecteval -export datasets-crops datasets/zone1-day datasets/zone1-night
```

This saves only the area watched in each cast, plus an `index.jsonl`, which is far smaller than the full frames. Keep the output out of git.
