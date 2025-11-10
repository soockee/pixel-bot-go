# Debug & Profiling Guide

This document explains how to enable runtime profiling to investigate memory leaks, CPU hotspots, and goroutine growth in Pixel Bot.

## Enabling Profiling
Set `"debug": true` in `pixle_bot_config.json` **before** starting the application. When enabled, an internal HTTP server exposes the standard Go `pprof` endpoints at:

```
http://localhost:6060/debug/pprof/
```
Disable by reverting `"debug": false`.

## Key Endpoints
| Endpoint                          | Purpose                                                  |
| --------------------------------- | -------------------------------------------------------- |
| `/debug/pprof/`                   | Index page listing available profiles                    |
| `/debug/pprof/profile?seconds=30` | CPU profile over interval (default 30s if not specified) |
| `/debug/pprof/heap`               | Heap allocations (live objects)                          |
| `/debug/pprof/goroutine`          | Goroutine dump (use `?debug=2` for human readable)       |
| `/debug/pprof/block`              | Blocking profile (contention)                            |
| `/debug/pprof/mutex`              | Mutex hold/wait times                                    |

## Common Commands (PowerShell)
```powershell
# CPU profile for 30 seconds (opens web UI)
go tool pprof -http=: http://localhost:6060/debug/pprof/profile?seconds=30

# Heap snapshot (in-use space)
go tool pprof -http=: http://localhost:6060/debug/pprof/heap

# Goroutine dump to file
curl http://localhost:6060/debug/pprof/goroutine?debug=2 > goroutines.txt

# Mutex profile (samples must accumulate; let app run under load)
curl http://localhost:6060/debug/pprof/mutex > mutex.pb

# Block profile (contention)
curl http://localhost:6060/debug/pprof/block > block.pb
```

## Interpreting Results
### Heap Profile
Look for large retained byte counts associated with:
- Frame buffers or image slices continually appended but never released
- Channels or slices growing unbounded
- Maps keyed by timestamps or IDs without eviction

Switch between views inside the pprof UI:
- `inuse_space` (currently retained)
- `alloc_space` (cumulative allocations; helps spot churn)

Comparing two heap snapshots:
```powershell
go tool pprof -diff_base pooled1.pb.gz pooled2.pb.gz
```

### Goroutine & Stack (Minimal)
Heap profiles exclude goroutine stacks. To rule out goroutine / stack-driven RSS growth, log just goroutine count and stack usage via runtime metrics + `MemStats`.

Add a short-lived debug loop:
```go
func DebugGoroutineStacks(interval time.Duration) {
    t := time.NewTicker(interval)
    defer t.Stop()
    samples := []metrics.Sample{{Name: "/sched/goroutines:goroutines"}}
    for range t.C {
        metrics.Read(samples)
        goroutines := samples[0].Value.Uint64()
        var ms runtime.MemStats
        runtime.ReadMemStats(&ms)
        log.Printf("goroutines=%d stackInuse=%dKB stackSys=%dKB heapAlloc=%dKB", goroutines, ms.StackInuse/1024, ms.StackSys/1024, ms.HeapAlloc/1024)
    }
}
```
Invoke when debugging:
```go
go DebugGoroutineStacks(1 * time.Second)
```
Interpretation (after warm-up):
- Stable `goroutines` + stable `stackInuse` ⇒ stacks not cause.
- Growing `goroutines` or `stackInuse` ⇒ investigate leaked goroutines or deep stack usage.
If RSS keeps rising with both stable, focus on native allocations (image/GDI/mmap) or unreleased arenas.

### CPU Profile
High percentages in functions like detection loops or scaling routines may indicate optimization opportunities: preallocation, reduced conversions, or avoiding redundant image processing.

### Goroutines
If goroutine count increases steadily without returning to a baseline, you may have leaked workers or unbounded background tasks. Inspect stack traces for repeatedly waiting on receive/send that never completes.

## Monitoring Process RSS (Working Set)
Heap profiles show only Go-managed heap objects. The process Working Set (RSS) can grow due to:
- Native allocations (GDI / DIB sections, Tk toolkit, cgo)
- Go heap arenas not yet returned to the OS (even if objects freed)
- Goroutine stacks and runtime structures
- Temporary large buffers causing high-water marks

Track RSS alongside heap to distinguish a true leak from allocator reuse.

### Quick Live Console Loop
```powershell
$botPid = (Get-Process -Name pixel-bot-go).Id
while ($true) {
	$p = Get-Process -Id $botPid -ErrorAction Stop
	$wsMB    = [math]::Round($p.WorkingSet64 / 1MB, 1)
	$privMB  = [math]::Round($p.PrivateMemorySize64 / 1MB, 1)
	$threads = $p.Threads.Count
	$handles = $p.HandleCount
	'{0:HH:mm:ss} WS={1}MB Private={2}MB Thr={3} Hnd={4}' -f (Get-Date), $wsMB, $privMB, $threads, $handles
	Start-Sleep 1
}
```

### Interpreting RSS vs Heap
- If `WS` keeps climbing but heap `inuse_space` snapshots stay roughly flat → native memory or unreleased arenas.
- If both climb together steadily past warm-up → real retention / leak path.
- Spikes with partial drops → transient large allocations (e.g. scaling buffers) and GC reclaim.

Combine with heap snapshots:
```powershell
Invoke-WebRequest http://localhost:6060/debug/pprof/heap -OutFile heap.pb.gz
go tool pprof heap.pb.gz
```

Look at difference between Working Set and `HeapAlloc` (from runtime stats) to gauge non-heap usage.


