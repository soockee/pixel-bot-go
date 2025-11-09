# Fishing State Machine & Bite Detection (Technical Details)

This document describes the internal finite state machine (FSM) used to automate the fishing workflow and the Phase 1 bite detection logic.

---
## Overview
The FSM cycles through phases. Two states (`casting`, `reeling`) are treated as **ephemeral**: their entry actions run immediately inside the state machine code (no external callbacks) and the machine advances to the next stable state without notifying listeners separately for the transient state.

| State        | Type      | Purpose                                                                      | Next (automatic)                     |
| ------------ | --------- | ---------------------------------------------------------------------------- | ------------------------------------ |
| `searching`  | stable    | Scan screen / selection for the bobber template.                             | On template -> `monitoring`          |
| `monitoring` | stable    | Watch locked coordinate for bite movement.                                   | Bite -> `reeling`; lost -> `casting` |
| `reeling`    | ephemeral | Perform reel input (mouse/key).                                              | Immediately -> `cooldown`            |
| `cooldown`   | stable    | Wait for loot/animation before recast (configurable via `cooldown_seconds`). | Timer expiry -> `casting`            |
| `casting`    | ephemeral | Execute cast input.                                                          | Immediately -> `searching`           |

Ephemeral entry actions:
- `casting`: executes cast key then transitions to `searching`.
- `reeling`: performs reel action, starts cooldown timer, transitions to `cooldown`.

Cooldown duration is configurable via `cooldown_seconds` in the JSON config (`pixle_bot_config.json`).

Listener callbacks now receive only `(prev, final)` where `final` is the resulting stable (or cooldown) state after any ephemeral hop. They do not see intermediate `casting` or `reeling` states directly. Input synthesis (mouse/key) is performed inline by the FSM using the configured key.

---
| Transition Triggers
| Event                        | Description                       | From -> To (listener receives)            |
| ---------------------------- | --------------------------------- | ----------------------------------------- |
| `EventTargetAcquiredAt(x,y)` | Template found; locks coordinate. | `searching` -> `monitoring`               |
| `EventTargetLost()`          | Monitoring lost motion; recast.   | `monitoring` -> `searching` (via casting) |
| `EventFishBite()`            | Bite confidence reached.          | `monitoring` -> `cooldown` (via reeling)  |
| `Tick(now)`                  | Periodic timer advance.           | `cooldown` -> `searching` (via casting)   |
| `ForceCast()`                | Manual cast command.              | any -> `searching` (via casting)          |
| `Reset()`                    | Clears state machine.             | any -> `searching`                        |

---
## Bite Detection (Visual Phase 1)
Implemented in `app/bite_detector.go`.

### ROI Formation
- On entering `monitoring`, locked coordinate `(x,y)` is recorded.
- Each frame a square ROI of side `roi_size_px` is clamped around `(x,y)` (accounting for selection offset if using partial capture).

### Frame Processing
1. Convert ROI to grayscale using integer luma approximation `(77R + 150G + 29B) >> 8`.
2. Compute absolute per-pixel difference vs previous grayscale frame.
3. Pixels with diff ≥ `diff_threshold` are motion pixels.
4. Vertical center-of-mass (COM) `y_cm` of motion pixels is computed; if no motion, carry previous COM (or ROI center for first frame).

### Bite Criteria
A bite triggers when all hold for the latest frame pair:
- Downward displacement `Δy = y_now - y_prev ≥ min_fall_pixels`.
- Velocity `v = Δy / Δt ≥ min_velocity_px_per_sec`.
- Time interval `Δt ≤ fall_window_ms`.
- Motion pixel counts in both frames ≥ `minDiff` (heuristic `max(8, min_fall_pixels/2)`).
- Optional smoothing: current `y_now` exceeds average of last `smoothing_frame_count` by at least `min_fall_pixels/2`.
- Cooldown: elapsed since `lastTrigger` ≥ `post_trigger_silence_ms`.

If satisfied: `EventFishBite()`.

### Target Lost Heuristic
Average diff pixel count over last 6 frames < 4 ⇒ invoke `EventTargetLost()` to resume searching.

### Configuration Parameters
Defined in `config.Config`:
- `roi_size_px`
- `diff_threshold`
- `min_fall_pixels`
- `fall_window_ms`
- `min_velocity_px_per_sec`
- `post_trigger_silence_ms`
- `smoothing_frame_count`
- `visual_standalone_factor` (reserved for future audio fusion)
- Future audio placeholders: `audio_enabled`, `audio_band_low_hz`, `audio_band_high_hz`, `audio_spike_factor`.

### Logging
On detection: log info with `deltaY`, `velocity`, `diffPrev`, `diffCurr`.

### Edge Cases & Guards
- Negative or zero `Δy`: cancels candidate.
- Excessively slow frame interval: may exceed `fall_window_ms` -> ignore.
- Very high diff counts from global movement: still filtered by ROI; future improvement could include a max diff cap.

---
## Future Audio Fusion (Phase 2 Planned)
Add secondary confirmation via WASAPI loopback capture:
1. Short audio buffers (≈50ms) processed for band-limited energy (e.g. splash frequencies).
2. Spike detection using rolling mean + σ multiple or absolute threshold.
3. Maintain `audioSpikeUntil` window; visual + audio within window ⇒ lower visual thresholds or earlier trigger.
4. Introduce confidence score `S = w1*Δy + w2*velocity + w3*audioSpikeMagnitude`.

---
## Possible Enhancements
- Adaptive threshold tuning based on false positive / miss counts.
- Persist performance metrics (avg detection latency, false triggers).
- Multi-frame ROI drifting if bobber moves slowly.
- GPU acceleration for diff computation.

---
## Testing Suggestions
- Record screen segment with simulated bobber drop to replay frames through detector.
- Unit test for `BiteDetector` using synthetic sequences: steady, small jiggle, valid drop.
- Benchmark diff loop for varying ROI sizes.

---
## Summary
The FSM separates search, monitoring, and cooldown as stable phases while treating casting and reeling as instantaneous entry actions implemented internally (no injected callbacks). Listeners observe only stable resulting states, simplifying UI and logging logic. The bite detector adds lightweight motion analysis constrained to a small ROI for performance.
