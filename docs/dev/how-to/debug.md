---
title: "Debug and profile"
summary: "Turn on debug mode, read live state, take profiles and watch memory while the app runs."
order: 6
---

## Enable Debug Mode

Set `"debug": true` in `pixle_bot_config.json` then start the app. Visit `http://localhost:6060/debug/pprof/` for profiles. Disable via `"debug": false`. Debug mode switches on the developer tools (pprof, `/debug/state`, DevTools, the Recorder tab and the recording settings); the log holds the Debug lines, such as `splash seen`, `spray under the bar` (a cast's spray that stayed under the splash bar, each new hundredth from 0.04), `sound without a splash` and `detection.search`, in every mode. A Warn `ESP32 recovered` (`action`, `cause` "reply lost" or "Bluetooth link dropped") marks an ESP32 action the run got past instead of halting.

## Collect CPU / Heap Profiles

1. Start capture so detection runs.
2. Hit CPU profile endpoint (`/debug/pprof/profile?seconds=30`).
3. Fetch heap snapshot (`/debug/pprof/heap`).
4. Diff snapshots later to identify growth.

## Monitor Goroutines & Memory

With debug enabled, compare `/debug/pprof/goroutine` and `/debug/pprof/heap` over time. Look for monotonic goroutine growth or heap growth.

## Leak Investigation Flow

1. Reproduce scenario for N minutes.
2. Capture baseline heap profile.
3. After interval capture second profile.
4. Diff—focus on functions retaining additional memory.
5. Correlate with goroutine logs.
6. If heap stable but RSS rising, inspect native allocations/UI buffers.

## Watch process memory

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
