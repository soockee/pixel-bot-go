# Fishing State Machine (Concise Guide)

This guide explains the finite state machine (FSM) that drives the fishing automation. It is aimed at beginner CS grads: basic familiarity with states/events is enough. Complex historical features (RGB matching, audio fusion) have been removed or deferred.

---
## 1. States at a Glance

| State           | What It Means                                                                                     | How You Leave It                                                  |
| --------------- | ------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------- |
| `halt`          | Bot is idle; no timers running.                                                                   | Enable capture → `waiting_focus`                                  |
| `waiting_focus` | Waiting for the selected game window to gain focus.                                               | Focus acquired → `searching`                                      |
| `searching`     | Looking for the bobber template (scanning scales). A 5s timer will auto‑cast if nothing is found. | Found template → `monitoring`; timeout or forced cast → `casting` |
| `monitoring`    | Locked onto bobber coordinates; watching motion for a bite.                                       | Bite → `reeling`; bobber lost → `casting`                         |
| `reeling`       | Performs reel action (cursor move + right click). Immediately sets cooldown timer.                | Transition completes → `cooldown`                                 |
| `cooldown`      | Waiting before next cast (configured seconds). Timer expiry triggers a cast.                      | Cooldown timer → `casting`                                        |
| `casting`       | Sends cast key. After showing this state briefly it returns to `searching`.                       | Immediate internal transition → `searching`                       |

Notes:
* `casting` and `reeling` are short but now visible (listeners see their transitions).
* Timers, not external ticks, drive recast and search timeout.

---
## 2. Core Events

| Event                                                  | Typical Source                            | Effect                                         |
| ------------------------------------------------------ | ----------------------------------------- | ---------------------------------------------- |
| `EventAwaitFocus()`                                    | Capture enabled                           | `halt` → `waiting_focus`                       |
| `EventFocusAcquired()`                                 | Focus watcher (window title match)        | `waiting_focus` → `searching`                  |
| `EventTargetAcquired()` / `EventTargetAcquiredAt(x,y)` | Detection finds template                  | `searching` → `monitoring` (locks coordinates) |
| `EventTargetLost()`                                    | Bite detector heuristic (motion vanished) | `monitoring` → `casting`                       |
| `EventFishBite()`                                      | Bite detector motion pattern              | `monitoring` → `reeling` → `cooldown`          |
| `ForceCast()`                                          | User command or internal timer            | current (except casting) → `casting`           |
| `EventHalt()`                                          | User stops capture                        | any → `halt` (clears timers)                   |
| Internal search timer (5s)                             | FSM                                       | If still `searching` → `ForceCast()`           |
| Internal cooldown timer                                | FSM                                       | When time elapses → `ForceCast()`              |

Timers push a `ForceCast` event into the FSM’s channel; normal event handling then performs the cast key action.

---
## 3. Typical Loop Sequence

```
waiting_focus → searching → monitoring → reeling → cooldown → casting → searching → ...
```
If the bobber is never found early, the search timeout fires: `searching → casting → searching`.

---
## 4. Design Choices (Why This Shape?)
* Separate `searching` vs `monitoring` keeps detection cost low once position is known.
* Timers remove dependence on irregular GUI ticks, making cast cadence predictable.
* Showing `casting` / `reeling` helps the UI and logs reflect real actions for debugging.
* A single event channel serializes state changes, avoiding race conditions with timers.

---
## 5. Bite Detection (Short Overview)
While in `monitoring`, a small square ROI around the locked bobber coordinate is converted to grayscale each frame. Motion differences and vertical displacement heuristics decide if a “bite” occurred (downward drop + velocity + minimal noise). A valid bite triggers `EventFishBite()`.

Lost target heuristic: too little motion over recent frames ⇒ `EventTargetLost()` (returns to casting sooner).

---
## 6. Extensibility Hooks
Ideas you could add later:
* Adaptive threshold based on recent misses.
* Alternate casting strategies (e.g. variable cooldown).
* Secondary confirmation (audio splash) integrated as another event source.

---
## 7. Mental Model Summary
Think of the FSM as a cycle with two timer gates:
* Search gate (5s) ensures progress even if detection fails.
* Cooldown gate ensures reel animations finish before next cast.

Everything else is event driven by detection or user input.

If you understand states + events + timers here, you can trace any fishing cycle by reading the log of transitions.

