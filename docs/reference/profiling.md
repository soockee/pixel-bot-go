---
title: "Profiling endpoints"
summary: "The pprof and debug endpoints, and common fixes for what they show."
order: 10
---

## Profiling Endpoints (pprof)

| Endpoint                        | Purpose            |
| ------------------------------- | ------------------ |
| /debug/pprof/                   | Index page         |
| /debug/pprof/profile?seconds=30 | CPU profile window |
| /debug/pprof/heap               | Heap allocations   |
| /debug/pprof/goroutine          | Goroutine stacks   |
| /debug/pprof/block              | Blocking profile   |
| /debug/pprof/mutex              | Mutex contention   |

## Common Fix Patterns

Reuse buffers; tune scale range; batch work instead of per-op goroutines; introduce backpressure via bounded channels.
