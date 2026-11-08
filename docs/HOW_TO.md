# How-To Guides

Procedural tasks and tuning steps extracted from detection, profiling, and operational docs.

---
## Enable Debug Mode
Set `"debug": true` in `pixle_bot_config.json` then start the app. Visit `http://localhost:6060/debug/pprof/` for profiles. Disable via `"debug": false`.

## Collect CPU / Heap Profiles
1. Start capture so detection runs.
2. Hit CPU profile endpoint (`/debug/pprof/profile?seconds=30`).
3. Fetch heap snapshot (`/debug/pprof/heap`).
4. Diff snapshots later to identify growth.

## Monitor Goroutines & Memory
With debug enabled, compare `/debug/pprof/goroutine` and `/debug/pprof/heap` over time. Look for monotonic goroutine growth or heap growth.

## Tune Detection Speed vs Accuracy
| Symptom             | Adjust                                |
| ------------------- | ------------------------------------- |
| False positives     | Raise `Threshold`, enable `Refine`    |
| Slow detection      | Narrow the scale range                |
| Miss different size | Expand `MinScale..MaxScale`           |
| Memory spike        | Reduce scale count / reuse buffers    |

## Observability & Metrics
Enable debug → inspect per‑scale timing to see if one scale dominates. If refinement always costs little and improves accuracy, keep it on; if negligible improvement disable for marginal speed gain.

## Leak Investigation Flow
1. Reproduce scenario for N minutes.
2. Capture baseline heap profile.
3. After interval capture second profile.
4. Diff—focus on functions retaining additional memory.
5. Correlate with goroutine logs.
6. If heap stable but RSS rising, inspect native allocations/UI buffers.

## Monitoring Process RSS (PowerShell Loop)
```powershell
$botPid = (Get-Process -Name pixel-bot-go).Id
while ($true) {
  $p = Get-Process -Id $botPid -ErrorAction Stop
  $wsMB = [math]::Round($p.WorkingSet64 / 1MB, 1)
  $privMB = [math]::Round($p.PrivateMemorySize64 / 1MB, 1)
  '{0:HH:mm:ss} WS={1}MB Private={2}MB' -f (Get-Date), $wsMB, $privMB
  Start-Sleep 1
}
```

## Reproduce the Response Benchmark
Commit your changes first, so the manifest ties the run to one revision. Then run:

```bash
go run ./cmd/responsebench -out responsebench-results/my-run
```

The run takes a few seconds, and it refuses to write into an existing directory. Compare `CHECKSUMS.txt` with the reference run; identical lines mean identical bytes. To check saved files against their checksums, run `sha256sum -c CHECKSUMS.txt` inside the result directory.

To see how the frozen controllers were chosen, run `go run ./cmd/responsebench -split dev -out responsebench-results/dev`. Do not change `frozen` in `cmd/responsebench/config.json` after looking at eval results; `TestFrozenSettingsMatchDevSelection` fails if it no longer matches the dev selection. For a variation, copy the file, edit the copy, pass it with `-config`, and report that run as exploratory.

### Run streaming trace feedback

```bash
go run ./cmd/responsebench -trace-feedback -split dev -out responsebench-results/trace-feedback-dev
go run ./cmd/responsebench -trace-feedback -split eval -out responsebench-results/trace-feedback-eval
```

Use `-config` to supply scenario families and scorer settings, or `-seeds 200000,200001` to run a subset of the selected split. During development, choose settings using only the dev split, then freeze the config for evaluation. Read `summary.md` for the paired results and `metrics.jsonl` for each signal, its supporting event IDs, and any controller adjustment.

The default config also enables two independent detectors in this mode. Read `shadow-model.json` for fitted parameters and calibration cutoffs, `metrics.jsonl` for per-profile alerts, and `shadow-reference.jsonl` for false alarms on separate synthetic reference runs. Compare the backoff profiles in the paired results and check the `mixed` workload separately. That workload is excluded from fitting and calibration by default.

### Inspect app dry-run replay alerts

From the repository root in PowerShell:

```powershell
$env:RESPONSEBENCH_REPLAY_OUT = 'responsebench-results/app-dry-run-replay'
go test ./cmd/responsebench -run '^TestFakeGameReplayFeedsDryRunAttemptsToScorer$' -count=1 -v
Remove-Item Env:RESPONSEBENCH_REPLAY_OUT
```

Use a fresh output directory. The replay feeds rendered quiet and splash frames to the real fishing state machine and runs its cast and reel callbacks through the app's dry-run action interface. Both independent detectors observe that trace on Go's test clock. The test also runs as part of `go test ./...`; the environment variable enables file export.

Use `dry-run-alerts.jsonl` to find the alert time, detector score, cutoff, triggering record, and contributing cast IDs. Match those IDs to `event_id` in `dry-run-trace.jsonl`. The export also includes the detector summary and frozen model. A `dry_run_dispatched` outcome records callback execution only. This replay has no server replies or live retry outcomes.

### Poll dry-run alerts while the app runs

Use a frozen `shadow-model.json` from a responsebench or replay run. In PowerShell, start the app in one terminal and query the loopback endpoint from another:

```powershell
go run . -dry-run -trace-model responsebench-results/shadow-detectors-eval/shadow-model.json
```

```powershell
Invoke-RestMethod http://localhost:6060/debug/state | ConvertTo-Json -Depth 8
```

Select the local simulator in the app and start capture. The response includes the usual FSM fields and a `trace_observer` object with both detectors' totals and the last 32 alerts. Each alert has its elapsed time, score, cutoff, triggering record, and cast IDs. The model flag enables debug for that process and requires dry-run.

## Install the Bobber Marker Addon
The bot finds your bobber by the brackets the Bobber Marker addon draws around it while you fish. Link the addon into the game, so a `git pull` updates it:

```powershell
New-Item -ItemType Junction -Path "C:\Program Files (x86)\World of Warcraft\_forever_\Interface\AddOns\BobberMarker" -Target "$PWD\addon\BobberMarker"
```

Restart the game or type `/reload`. In NaowhForever's Glimmer settings, switch off the Fishing Bobber highlight, because both addons use the same nameplate. Then cast: magenta brackets appear around your bobber and vanish when you reel. If the bobber is not in the middle of the box, move it with `/bobbermarker y <pixels>`; negative moves it down, and `/bobbermarker` shows the current value.

### Set the Fishing cast key

Open **Settings → Keys** in Visual Response Lab. Click the current **Cast key**, then press the same key or mouse side button you use to cast Fishing in game. The app saves it automatically. Use an unmodified letter, top-row digit, F1–F12, Mouse 4, or Mouse 5.

## Sign In
Fishing with real input and uploading recordings need a Google account with a role; a dry run needs none.

1. Click **Sign in with Google** in the top bar. The browser opens Google sign-in; pick your account and return to the app.
2. The top bar shows your email and role. Without a role, **Start fishing** stays disabled: ask the owner to give your account a role that may fish, then sign out and in again.

The app keeps you signed in (`pixel_bot_account.bin`, encrypted for your Windows user) and checks your role on each start. **Sign out** forgets it and stops a real run. Sign-in needs `cloud_url` and `oauth_client_id` in the config.

## Update the App
At each start a release build asks GitHub for the latest release of `soockee/pixel-bot-go`. When it is newer, the top bar shows **Update to vX.Y.Z**.

1. Stop the run, the recording and any upload; the button stays disabled until then.
2. Click **Update to vX.Y.Z**. The status bar shows the download; the app checks the build against the release's `CHECKSUMS.txt`, refuses it on a mismatch, then replaces itself and restarts with the same flags.

The replaced build stays beside the exe as `pixel-bot-go.exe.old` until the next start removes it. Local builds (`go build`) report version `dev` and never check. To try a version, build with `-ldflags "-X github.com/soocke/pixel-bot-go/internal/update.Version=v1.0.0"`.

## Record a Session
A recording saves the screen and your input together, on the same clock, and sends no input. The Recorder tab is a developer tool: it appears with `"debug": true` in `pixle_bot_config.json`.

1. Start the app. On the **Fishing** tab, pick the game window (the list reloads when you open it).
2. Open the **Recorder** tab and click **Record**. The new recording opens and follows your input live.
3. Focus the game and play by hand.
4. Press the stop hotkey (default F10), or click **Stop recording** in the app.

Each recording gets a new timestamped folder holding `frames/`, `input.jsonl` and the game's sound in `audio.wav`. You still hear the game while it records, and music or voice chat from other programs is not recorded. Set **Recording folder** and **Record keys** on the **Settings** tab (debug mode). The default `datasets/` folder is ignored by git. Start stays disabled while recording, and Record while the bot runs.

For scripts and agents, the same recording runs without a window: `pixel-bot-go.exe -record <dir> -window "<title>"`, stopped with Ctrl+C. It records the window's client area where it is at the start. Add `-no-keys` to skip keys. `-list-windows` prints the titles.

The recorder stores client-area cursor positions, relative mouse counts, both wheel axes, and press and release events for left, right, middle, Mouse 4, and Mouse 5. It also records key down and up with virtual-key and scan codes, repeat status, and shortcuts such as Shift+B. Mouse capture requires the pointer over the selected client area. Keyboard capture follows the selected foreground window, including its chat or text fields. Use `-no-keys` for mouse-only sessions. Keyboard input Windows marks as injected is skipped.

The JSONL keeps the raw button transitions and adds one gesture per completed hold. A hold becomes a drag if it moves at least 6 client pixels or reaches at least 8 raw counts away from its start. Gesture rows include duration, endpoints, total raw travel, and maximum excursions. A right-button camera drag can be detected even with a stationary cursor. Holds cut off by a capture gap are marked interrupted. A final `session_end` row reports the event counts and whether the file finished cleanly. The recorder captures input and key bindings. Bite and cast annotations are separate.

### Upload a recording

Open a finished recording on the **Recorder** tab and click **Upload**. It needs a role with `recordings.upload`. The status bar counts the files; anything already uploaded is skipped, so start it again after a failure. Uploaded files cannot be replaced.

### View a recording

The **Recorder** tab lists the recordings in the recording folder, newest first. Pick one to see its counts, the latest frame, the cursor map (movement density, completed click locations and the recent path), the raw right-button camera motion, the activity of the last minute and a log of gestures and shortcuts. A recording in progress updates live. Dry-run and fake-game traces in a recording folder show their cast and bite counts instead.

### Evaluate detectors

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

### Add and try a technique

1. Implement `detect.Locator` or `detect.BiteDetector`.
2. Add one line for it in `domain/detect/registry.go`.
3. Run `detecteval`. The new technique appears as a column.
4. To use it in the app, set `"debug": true` and pick it on the **Settings** tab, under **Locator** or **Bite detector**.

### Export data for training

```powershell
go run ./cmd/detecteval -export datasets-crops datasets/zone1-day datasets/zone1-night
```

This saves only the area watched in each cast, plus an `index.jsonl`, which is far smaller than the full frames. Keep the output out of git.

## Collect and compare playtest traces

Start each command in a separate PowerShell terminal from the repository root. Use a fresh session label and output paths. Start the fake game first so the recorder can find its window.

```powershell
go run ./cmd/fakegame -screen 2 -trace-session session-001 -trace-out fishing-recordings/game-001.jsonl
go run . -dry-run -screen 2 -auto-window "Fake Game" -trace-session session-001 -trace-out fishing-recordings/app-001.jsonl
.\pixel-bot-go.exe -record fishing-recordings/session-001 -window "Fake Game" -trace-session session-001
```

Set the app's capture region to the fake game and start capture. Keep the fake game focused during the session. Close the app and fake game normally and stop the recorder with Ctrl+C to finalize their traces. The fake game's bites are scripted.

```powershell
go run ./cmd/responsebench -seeds 200000 -out responsebench-results/compare-001
go run ./cmd/responsebench -compare -manual-cast mouse:x1 -manual-reel mouse:right fishing-recordings/session-001/input.jsonl fishing-recordings/game-001.jsonl fishing-recordings/app-001.jsonl responsebench-results/compare-001/playtrace.jsonl
```

Set the manual mappings to your bindings, for example `key:Shift+B` or `mouse:x2`, and add `-json` before the trace paths for machine-readable output. Omitting a mapping leaves those manual actions unlabeled. To inspect a schema 3 trace, put it as `input.jsonl` in a folder inside the recording folder and open it on the **Recorder** tab.

## Next Steps When Stuck
Investige profile `.pb.gz` files; Compare both baseline and later snapshots plus goroutine counts. use top command to identify hotspots or retention paths

## Quick Setup Recap
1. Configure detection scales conservatively.
2. Run capture.
3. Observe timings and adjust stride/threshold.
4. Enable debug only when profiling (minor overhead).

## Operating Tips
* Always run as Administrator for input injection reliability.
* Keep game window focused; alt-tab can delay key events.
* Play in a smaller window for faster searches; watching for a bite captures only the bobber's box either way.
* Pause before changing settings; apply then resume.
* Use dark mode if the UI is distracting in low light.

## FAQ (Quick Reference)
| Question                 | Answer                                               |
| ------------------------ | ---------------------------------------------------- |
| Cursor doesn't move      | Run as Administrator; confirm cast key binding.      |
| Never finds bobber       | Lower `Threshold`; ensure scale range brackets size. |
| Triggers on wrong pixels | Raise `Threshold`; refine ON. |
| Wants to fish forever    | Check `max_cast_duration_seconds` isn't too large.   |
| Reset config             | Delete `pixle_bot_config.json` while app closed.     |

---
## Source Attribution
Procedural sections originally in separate detection, debug, and state machine guides have been merged here.
