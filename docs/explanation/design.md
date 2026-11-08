---
title: "Design"
summary: "Why the state machine, the event loop and the input safety rules look the way they do."
order: 2
---

## FSM Lifecycle

```
waiting_focus → searching → monitoring → reeling → cooldown → casting → searching → …
```
A search deadline forces progress. Monitoring is lighter than searching because coordinates are known and only a small ROI is watched.

## Concurrency Snapshot

Core goroutines/tasks typically active during operation:
* Engine (one loop: grab a frame, run detection, feed the FSM)
* Scale match workers (spawn per scale; short-lived)
* FSM event loop (state transitions, deadline, input)
* Focus watcher (active from Start until the run halts)
* Optional debug loggers (goroutines & memory)

Use profiling endpoints to confirm counts remain stable; runaway worker spawning indicates misconfigured scale range.

## Design Rationale

Separate searching vs monitoring → lower cost post‑lock. Timers decouple cadence from UI ticks. Short explicit states (`casting`, `reeling`) improve observability. Single event channel serializes transitions avoiding races. The Engine runs without the UI, so a slow preview cannot delay a bite, and tests can replay the whole pipeline.

## Input Safety

Input only goes out while it can land where it should. Every state has at most one deadline, and leaving the state cancels it, so nothing pending can act after Stop or a focus change. All input runs on the FSM goroutine. Outside dry-run, `action.Guard` checks each action: the selected window must be in front, moves must stay inside the game window, and the pointer must still be where the app left it. If it has moved, the user has taken over and the run halts. A run also halts on the stop hotkey, after too many failed casts in a row, and at the session limit.

## Extensibility Ideas

Adaptive thresholds, variable cooldown strategies, multi‑modal confirmation (audio splash) can be added as new event sources feeding the FSM.

The `splash` bite detector watches the centre of the marker box, where the bobber sits. A bite throws up a splash there, which brightens many centre pixels within one or two frames. Spell effects of other players and passing light spread across the whole box, so the detector needs at least half of the brightened pixels in the centre. Each pixel is compared with a baseline that takes on the scene with a 0.5 s lag, so lasting changes such as the landed bobber or a parked cursor blend in, while the splash appears within a frame or two. It ignores the first 3 s after the lock, while the bobber lands.

The `splash_sound` bite detector adds the game's sound. In recordings the bite sound starts 0.2 to 0.45 s after the splash at the player's bobber. Other players' bobbers make the same sound but splash elsewhere, and spell effects splash without it, so the detector needs both: a sound starting within 0.8 s after a splash in the centre of the box. A sound starts when its 50 ms level is above 15 and three times the median of the 2 s before. The engine taps the selected window's process through Windows process loopback, which leaves the game audible and records no other program. The game mutes itself in the background unless its sound-in-background option is on, but the bot only fishes while the game is focused.

## Limitations

Lighting shifts, motion blur, busy backgrounds, rotation/perspective changes, extreme scale ranges degrade accuracy or performance. The approach is 2D template matching + light motion heuristics (not full CV/ML).
