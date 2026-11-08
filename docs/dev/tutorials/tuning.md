---
title: "Tune the template search"
summary: "Trade search speed against accuracy step by step, and make bite detection steadier."
order: 3
---

## Speed without losing accuracy

1. Start with stride 2–4, refine ON.
2. Observe average time spent in `searching` (logs or UI).
3. Increase stride by 1; if misses rise, revert or keep refine ON.
4. Narrow scale range if object size stable (remove extremes).
5. Stop when search latency and accuracy acceptable.

## Steadier bite detection

1. Verify stable template lock (few `TargetLost` events).
2. Slightly enlarge ROI if motion spikes missed.
3. Ensure steady lighting; avoid UI overlays near bobber.
4. If premature bites trigger, raise motion spike threshold (future config item) or raise `Threshold` to improve initial lock quality.
