---
title: "Collect and compare playtest traces"
summary: "Record traces from dry runs and real play, and compare them with the benchmark."
order: 8
---

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
