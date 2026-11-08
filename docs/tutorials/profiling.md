---
title: "Profile a run"
summary: "Take CPU and heap profiles during a run and look into memory that keeps growing."
order: 4
---

## A first profile

1. Enable `"debug": true`.
2. Run capture for 30s while searching & monitoring states cycle.
3. Gather CPU profile & heap snapshot.
4. Identify top hotspots; if NCC dominates, adjust stride/scale before code changes.
5. Disable debug mode afterwards.

## Memory that keeps growing

1. Baseline heap profile.
2. Run 5–10 minutes with varied states.
3. Second heap profile; diff.
4. If retained growth isolated to scaled template allocations, reduce scale range.
5. If goroutines keep climbing, inspect detection worker spawning logic.
