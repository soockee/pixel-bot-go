# Debug & Profiling Guide (Beginner-Friendly)

This guide shows you, a CS beginner/early grad, how to investigate performance and memory issues in Pixel Bot. You don't need deep systems knowledge—just follow the steps.

---
## 1. What "Debug" Mode Does
Set `"debug": true` in `pixle_bot_config.json` before launching. When enabled:
1. A small HTTP server exposes Go's built‑in profiling endpoints (`pprof`).
2. Optional in‑process loggers can write periodic memory and goroutine stats (disabled by default unless wired in `app/app.go`).

Browse:
```
http://localhost:6060/debug/pprof/
```
Turn it off by setting `"debug": false`.

---
## 2. Concurrency Snapshot: What Runs in Goroutines?
Recent changes split subsystems into their own goroutines. Typical background tasks:
- Capture pipeline (grabs frames).
- Multi‑scale detection workers (spawned per scale factor; short‑lived).
- Fishing FSM event loop (state transitions, timers).
- Focus watcher (only while waiting for game window focus).
- Optional metrics loggers (goroutines + memory stats).

Seeing more goroutines than expected? Check if detection scale settings exploded (very small `ScaleStep` spanning wide `MinScale..MaxScale`).

---
## 3. Key pprof Endpoints
| Endpoint                          | Purpose                                         |
| --------------------------------- | ----------------------------------------------- |
| `/debug/pprof/`                   | Index page (links to all profiles)              |
| `/debug/pprof/profile?seconds=30` | CPU profile over N seconds (default 30)         |
| `/debug/pprof/heap`               | Heap (live allocations)                         |
| `/debug/pprof/goroutine`          | Goroutine stacks (`?debug=2` for readable text) |
| `/debug/pprof/block`              | Blocking (goroutines waiting on sync)           |
| `/debug/pprof/mutex`              | Mutex hold / contention                         |

Tip: Collect CPU profiles while the bot is actively detecting (capture on, searching or monitoring state), not idle.

---
## 4. Handy PowerShell Commands
```powershell
# CPU profile (30s) and open local web explorer
go tool pprof -http=: http://localhost:6060/debug/pprof/profile?seconds=30

# Heap snapshot
go tool pprof -http=: http://localhost:6060/debug/pprof/heap

# Goroutine dump (human readable)
curl http://localhost:6060/debug/pprof/goroutine?debug=2 > goroutines.txt

# Mutex / block samples (capture while app under load)
curl http://localhost:6060/debug/pprof/mutex > mutex.pb
curl http://localhost:6060/debug/pprof/block > block.pb
```

Diff two heap snapshots to see growth patterns:
```powershell
go tool pprof -diff_base before.pb.gz after.pb.gz
```

---
## 5. Reading a Heap Profile (Plain Language)
Look for functions holding a lot of bytes (retained memory). Usual suspects if something is off:
- Frame or template buffers that never get reused.
- Large slices grown in loops (e.g. many scales) without pooling.
- Channels or maps that keep accumulating entries.

Switch views inside the UI:
- `inuse_space`: current retained memory.
- `alloc_space`: total allocated over time (reveals churn).

If retained memory is stable after a few minutes of normal activity, you're likely fine.

---
## 6. Goroutines & Stack Usage
Why it matters: Even if heap is stable, runaway goroutines or very deep stacks can push overall memory up.

The project now offers lightweight loggers:
```go
debug.StartGoroutineLogger(2 * time.Second, logger)
debug.StartMemLogger(2 * time.Second, logger)
```
Uncomment their calls in `app/app.go` when `cfg.Debug` is true to enable.

Interpret logs (simplified):
- `goroutines` rising forever ⇒ leak (unbounded spawning).
- `stack_inuse` growing with stable heap ⇒ goroutine stack pressure.
- Stable goroutines + rising RSS (`rss`) ⇒ likely native/UI allocations (Tk/windowing) or image buffers.

---
## 7. CPU Profile Quick Checks
Open the CPU profile flame graph:
- Hot spots in NCC loops (`matchTemplateNCCGrayIntegralPre`) or scaling (`getScaledTemplatePrecompFromBase`) ⇒ consider lowering number of scales or increasing `Stride`.
- High time in GC (`runtime.gc`) ⇒ many short‑lived allocations; look for temporary slices you can reuse.

Beginner tip: Optimize only after measuring. A fast wrong guess wastes time.

---
## 8. Monitoring Whole Process Memory (RSS)
Heap != RSS. Working Set includes native allocations and OS pages.

Quick watch loop:
```powershell
$botPid = (Get-Process -Name pixel-bot-go).Id
while ($true) {
    $p = Get-Process -Id $botPid -ErrorAction Stop
    $wsMB   = [math]::Round($p.WorkingSet64 / 1MB, 1)
    $privMB = [math]::Round($p.PrivateMemorySize64 / 1MB, 1)
    '{0:HH:mm:ss} WS={1}MB Private={2}MB' -f (Get-Date), $wsMB, $privMB
    Start-Sleep 1
}
```

Interpretation cheat sheet:
- WS steady, HeapAlloc steady ⇒ OK.
- WS grows, HeapAlloc flat ⇒ native/UI allocations or allocator high watermark.
- Both grow together ⇒ real retention path (investigate heap profile owners).
- Sawtooth pattern ⇒ large temporary buffers freed by GC (normal).

---
## 9. Practical Leak Investigation Flow
1. Enable debug mode, reproduce suspected leak for a few minutes.
2. Capture heap profile (#1) and note HeapAlloc number.
3. Wait N minutes, capture heap profile (#2).
4. Diff profiles. Are the same functions holding more memory? Follow the call path.
5. Check goroutine log output for growth.
6. If heap stable but RSS rising: enable `StartMemLogger` and look at `rss` vs `heap_alloc`. Focus on native sources.

---
## 10. Common Fix Patterns (Plain)
- Reuse buffers (keep a slice, resize instead of reallocate).
- Limit number of scales (tune `MinScale`, `MaxScale`, `ScaleStep`).
- Avoid spawning a goroutine per trivial operation; batch work.
- Add backpressure (bounded channels) if producers can outpace consumers.

---
## 11. Glossary (Short)
- Heap: Go-managed memory for objects.
- RSS (Working Set): Total resident pages for the process (heap + native + stacks).
- Goroutine: Lightweight concurrent function; many can exist.
- Profile: Snapshot of runtime data (CPU samples, heap allocations, etc.).
- Stride: Step size when scanning; larger stride = faster, less precise.

---
## 12. Next Steps
If you get stuck interpreting a profile, save the `.pb.gz` file and ask for help: "What are the top retainers?" Provide both baseline and later snapshots.

That’s all—you now have a repeatable way to check performance and track down leaks.


