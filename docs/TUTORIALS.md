# Tutorials

Hands-on walkthroughs to achieve common first wins. Derived from existing detection and debugging material.

---
## 1. First Successful Fishing Cycle
1. Launch app; ensure game window visible.
2. (Optional) Set a Selection Area around bobber region.
3. Start capture; watch state: `waiting_focus → searching`.
4. When `monitoring` appears, a match was found; observe ROI stability.
5. Wait for bite (cursor moves & reels) then cooldown → automatic recast.
6. Adjust `Threshold` only if false triggers occur or misses persist.

Outcome: One full automatic cast → bite → reel → recast loop.

## 2. Speed Tuning Without Losing Accuracy
1. Start with stride 2–4, refine ON.
2. Observe average time spent in `searching` (logs or UI).
3. Increase stride by 1; if misses rise, revert or keep refine ON.
4. Narrow scale range if object size stable (remove extremes).
5. Stop when search latency and accuracy acceptable.

## 3. Basic Profiling Session
1. Enable `"debug": true`.
2. Run capture for 30s while searching & monitoring states cycle.
3. Gather CPU profile & heap snapshot.
4. Identify top hotspots; if NCC dominates, adjust stride/scale before code changes.
5. Disable debug mode afterwards.

## 4. Investigate Suspected Memory Growth
1. Baseline heap profile.
2. Run 5–10 minutes with varied states.
3. Second heap profile; diff.
4. If retained growth isolated to scaled template allocations, reduce scale range.
5. If goroutines keep climbing, inspect detection worker spawning logic.

## 5. Improving Bite Detection Reliability
1. Verify stable template lock (few `TargetLost` events).
2. Slightly enlarge ROI if motion spikes missed.
3. Ensure steady lighting; avoid UI overlays near bobber.
4. If premature bites trigger, raise motion spike threshold (future config item) or raise `Threshold` to improve initial lock quality.

## 6. Minimal Troubleshooting Flow
| Symptom            | Action                                            |
| ------------------ | ------------------------------------------------- |
| No matches         | Lower `Threshold`, verify scale range covers size |
| Many false matches | Raise `Threshold`, keep refine enabled            |
| Slow scan          | Increase `Stride`, narrow scales                  |
| Memory pressure    | Reduce scale count; profile heap                  |

---
## Source Attribution
Tutorial steps synthesized from earlier standalone detection, state machine, and debug guides now consolidated.
