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
Enable in code (when `cfg.Debug`):
```go
debug.StartGoroutineLogger(2 * time.Second, logger)
debug.StartMemLogger(2 * time.Second, logger)
```
Look for monotonic goroutine growth or RSS divergence.

## Tune Detection Speed vs Accuracy
| Symptom             | Adjust                                |
| ------------------- | ------------------------------------- |
| False positives     | Raise `Threshold`, enable `Refine`    |
| Slow detection      | Increase `Stride`, narrow scale range |
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
* Narrow selection area for speed & fewer false positives.
* Pause before changing settings; apply then resume.
* Use dark mode if the UI is distracting in low light.

## FAQ (Quick Reference)
| Question                 | Answer                                               |
| ------------------------ | ---------------------------------------------------- |
| Cursor doesn't move      | Run as Administrator; confirm reel key binding.      |
| Never finds bobber       | Lower `Threshold`; ensure scale range brackets size. |
| Triggers on wrong pixels | Raise `Threshold`; refine ON; narrow selection area. |
| Wants to fish forever    | Check `max_cast_duration_seconds` isn't too large.   |
| Reset config             | Delete `pixle_bot_config.json` while app closed.     |

---
## Source Attribution
Procedural sections originally in separate detection, debug, and state machine guides have been merged here.
