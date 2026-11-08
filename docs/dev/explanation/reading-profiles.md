---
title: "Reading profiles"
summary: "What heap, CPU and goroutine profiles of Go Fish usually tell you."
order: 3
---

## Profiling Interpretation (Heap / CPU)

Large retained memory usually from buffers not reused (frames, scaled templates). High GC time indicates allocation churn (temporary slices). Hot CPU spots in NCC or scaling suggest tuning scale range or stride before deeper optimization.

## CPU Profile Signals

Hot functions: `scoreAt`, `grayPrecomp.load`, or repeated conversions. Optimize only after measuring—avoid premature micro‑tuning.

## Goroutines & Stacks

Rising goroutine count without stabilization = leak. Growing `stack_inuse` with flat heap can still raise RSS. Stable goroutines + rising RSS implies native/UI allocations or image buffers.
