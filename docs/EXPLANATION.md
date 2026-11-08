# Explanation

Conceptual background for Visual Response Lab: detection theory, state machine design, profiling interpretation. This file consolidates earlier separate detection, state machine, and debug documents.

---
## Concurrency Snapshot
Core goroutines/tasks typically active during operation:
* Engine (one loop: grab a frame, run detection, feed the FSM)
* Scale match workers (spawn per scale; short-lived)
* FSM event loop (state transitions, deadline, input)
* Focus watcher (active from Start until the run halts)
* Optional debug loggers (goroutines & memory)

Use profiling endpoints to confirm counts remain stable; runaway worker spawning indicates misconfigured scale range.

---
## Why the Bobber Comes from the Addon
Real footage showed the template failing: the bobber is larger than the template, it changes with the camera, and other players' bobbers look the same. The addon already knows which object is ours and marks it, so reading its brackets is simpler and more reliable than recognising the object. It also gives the area to watch, which keeps the bite detector off the rest of the water. The cost is a dependency on the addon being on, with its colour unchanged.

The first addon used, NaowhForever's Glimmer, draws soft brackets blended additively with the scene, so their colour shifts from day to night, and its box sits about 20 px below the bobber. The project's own Bobber Marker draws opaque magenta in exact screen pixels, centred on the bobber, and only while Fishing is channelled. The locator then sees one fixed colour, the click lands on the bobber, the splash falls in the centre of the watched box, and no marker lingers after a reel to be locked on by the next cast.

## Detection Mental Model
Imagine laying the template on every possible position on the frame (like a stencil) and asking “How similar is this patch?” Keep the highest score. Brute force plus shortcuts (stride, integral images, early stop, parallel scales) keeps it fast.

## NCC Overview
Normalized cross‑correlation (NCC) outputs -1..+1. Near +1 = strong match, ~0 = unrelated. We normalize (subtract mean, divide by std) so lighting shifts don’t break matches.

## Multi-Scale Matching
Template is resized across a range (`MinScale..MaxScale` step `ScaleStep`) because apparent object size changes (UI scale, distance). Each scale matched; best overall retained. More scales = more work; refine pass tightens coordinates.

## FSM Lifecycle
```
waiting_focus → searching → monitoring → reeling → cooldown → casting → searching → …
```
A search deadline forces progress. Monitoring is lighter than searching because coordinates are known and only a small ROI is watched.

## Design Rationale
Separate searching vs monitoring → lower cost post‑lock. Timers decouple cadence from UI ticks. Short explicit states (`casting`, `reeling`) improve observability. Single event channel serializes transitions avoiding races. The Engine runs without the UI, so a slow preview cannot delay a bite, and tests can replay the whole pipeline.

## Input Safety
Input only goes out while it can land where it should. Every state has at most one deadline, and leaving the state cancels it, so nothing pending can act after Stop or a focus change. All input runs on the FSM goroutine. Outside dry-run, `action.Guard` checks each action: the selected window must be in front, moves must stay inside the game window, and the pointer must still be where the app left it. If it has moved, the user has taken over and the run halts. A run also halts on the stop hotkey, after too many failed casts in a row, and at the session limit.

## Bite Detection Concept
While monitoring, a grayscale ROI stream feeds statistical heuristics (EMA + window diffs). A spike or a jump from the baseline makes the Engine send `EventFishBite()`. If no bite comes, a deadline ends the cast after `max_cast_duration_seconds`.

## Extensibility Ideas
Adaptive thresholds, variable cooldown strategies, multi‑modal confirmation (audio splash) can be added as new event sources feeding the FSM.

The `splash` bite detector watches the centre of the marker box, where the bobber sits. A bite throws up a splash there, which brightens many centre pixels within one or two frames. Spell effects of other players and passing light spread across the whole box, so the detector needs at least half of the brightened pixels in the centre. Each pixel is compared with a baseline that takes on the scene with a 0.5 s lag, so lasting changes such as the landed bobber or a parked cursor blend in, while the splash appears within a frame or two. It ignores the first 3 s after the lock, while the bobber lands.

The `splash_sound` bite detector adds the game's sound. In recordings the bite sound starts 0.2 to 0.45 s after the splash at the player's bobber. Other players' bobbers make the same sound but splash elsewhere, and spell effects splash without it, so the detector needs both: a sound starting within 0.8 s after a splash in the centre of the box. A sound starts when its 50 ms level is above 15 and three times the median of the 2 s before. The engine taps the selected window's process through Windows process loopback, which leaves the game audible and records no other program. The game mutes itself in the background unless its sound-in-background option is on, but the bot only fishes while the game is focused.

## Summary Mental Model
Two timer gates (search, cooldown) guarantee forward motion; detection / bite events adjust path opportunistically. Understand states + events + timers to trace any cycle.

## Profiling Interpretation (Heap / CPU)
Large retained memory usually from buffers not reused (frames, scaled templates). High GC time indicates allocation churn (temporary slices). Hot CPU spots in NCC or scaling suggest tuning scale range or stride before deeper optimization.

## Goroutines & Stacks
Rising goroutine count without stabilization = leak. Growing `stack_inuse` with flat heap can still raise RSS. Stable goroutines + rising RSS implies native/UI allocations or image buffers.

## CPU Profile Signals
Hot functions: `scoreAt`, `grayPrecomp.load`, or repeated conversions. Optimize only after measuring—avoid premature micro‑tuning.

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
