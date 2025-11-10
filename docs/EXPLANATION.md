# Explanation

Conceptual background for Pixel Bot: detection theory, state machine design, profiling interpretation. This file consolidates earlier separate detection, state machine, and debug documents.

---
## Concurrency Snapshot
Core goroutines/tasks typically active during operation:
* Capture pipeline (frame acquisition)
* Scale match workers (spawn per scale; short-lived)
* FSM event loop (state transitions + timers)
* Focus watcher (active only before first lock)
* Bite detection ROI processor (while monitoring)
* Optional debug loggers (goroutines & memory)

Use profiling endpoints to confirm counts remain stable; runaway worker spawning indicates misconfigured scale range.

---
## Detection Mental Model
Imagine laying the template on every possible position on the frame (like a stencil) and asking “How similar is this patch?” Keep the highest score. Brute force plus shortcuts (stride, integral images, early stop, parallel scales) keeps it fast.

## NCC Overview
Normalized cross‑correlation (NCC) outputs -1..+1. Near +1 = strong match, ~0 = unrelated. We normalize (subtract mean, divide by std) so lighting shifts don’t break matches. Transparent template pixels are ignored.

## Multi-Scale Matching
Template is resized across a range (`MinScale..MaxScale` step `ScaleStep`) because apparent object size changes (UI scale, distance). Each scale matched; best overall retained. More scales = more work; refine pass tightens coordinates.

## FSM Lifecycle
```
waiting_focus → searching → monitoring → reeling → cooldown → casting → searching → …
```
Search timeout forces progress (`ForceCast`). Monitoring is lighter than searching because coordinates are known and only a small ROI is watched.

## Design Rationale
Separate searching vs monitoring → lower cost post‑lock. Timers decouple cadence from UI ticks. Short explicit states (`casting`, `reeling`) improve observability. Single event channel serializes transitions avoiding races.

## Bite Detection Concept
While monitoring, a grayscale ROI stream feeds statistical heuristics (EMA + window diffs). Spikes / drops trigger `EventFishBite()`. Low motion over recent frames triggers `EventTargetLost()` to recast sooner.

## Extensibility Ideas
Adaptive thresholds, variable cooldown strategies, multi‑modal confirmation (audio splash) can be added as new event sources feeding the FSM.

## Summary Mental Model
Two timer gates (search, cooldown) guarantee forward motion; detection / bite events adjust path opportunistically. Understand states + events + timers to trace any cycle.

## Profiling Interpretation (Heap / CPU)
Large retained memory usually from buffers not reused (frames, scaled templates). High GC time indicates allocation churn (temporary slices). Hot CPU spots in NCC or scaling suggest tuning scale range or stride before deeper optimization.

## Goroutines & Stacks
Rising goroutine count without stabilization = leak. Growing `stack_inuse` with flat heap can still raise RSS. Stable goroutines + rising RSS implies native/UI allocations or image buffers.

## CPU Profile Signals
Hot functions: `matchTemplateNCCGrayIntegralPre`, scaling helpers, or repeated conversions. Optimize only after measuring—avoid premature micro‑tuning.

## Limitations
Lighting shifts, motion blur, busy backgrounds, rotation/perspective changes, extreme scale ranges degrade accuracy or performance. The approach is 2D template matching + light motion heuristics (not full CV/ML).

---
## Concept Glossary
| Term         | Meaning                                           |
| ------------ | ------------------------------------------------- |
| Template     | Small target image to locate in frame             |
| Frame        | Full captured screenshot                          |
| Scale Factor | Resize multiplier applied to template             |
| Stride       | Pixel step size while scanning                    |
| NCC          | Normalized cross‑correlation similarity score     |
| ROI          | Focus patch after initial lock                    |
| FSM          | Finite State Machine governing fishing loop       |
| EMA          | Exponential moving average used in bite detection |
| RSS          | Resident Set Size (process working set)           |

---
## Source Attribution
Original conceptual sections sourced from earlier standalone detection, state machine, and debug guides (now consolidated).
