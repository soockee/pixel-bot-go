---
title: "Debug and profile"
summary: "Turn on debug mode, read live state, take profiles and watch memory while the app runs."
order: 6
---

## Enable Debug Mode

Set `"debug": true` in `pixle_bot_config.json` then start the app. Visit `http://localhost:6060/debug/pprof/` for profiles. Disable via `"debug": false`.

## Observability & Metrics

Enable debug → inspect per‑scale timing to see if one scale dominates. If refinement always costs little and improves accuracy, keep it on; if negligible improvement disable for marginal speed gain.

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
