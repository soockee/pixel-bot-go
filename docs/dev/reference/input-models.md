---
title: "Input timing and cursor models"
summary: "The recorded human timing and cursor movement models and their file formats."
order: 9
---

## Recorded Input Timing

`responsebench -input-timing` reads manual recordings and emits a JSON report. It reconstructs the press time from each completed mouse gesture. Casts can include a side-button drag, and reels exclude camera drags. The report lists all candidate reels per cast and stops pairing at focus and capture gaps. Existing `labels.json` values appear as review evidence only, since their bite times may come from a fixed offset.

```powershell
go run ./cmd/responsebench -input-timing -manual-cast mouse:x1 -manual-reel mouse:right datasets/20261108-012132 datasets/20261108-035820
```

To verify a cast, look at the frames and audio around its candidates and save a `timing-review.json` next to `input.jsonl`. Use the reported press timestamps in microseconds. The review supplies that session's bindings. The example timestamps are illustrative.

```json
{
  "cast_binding": "mouse:x1",
  "reel_binding": "mouse:right",
  "casts": [
    {"cast_us": 1000000, "outcome": "reeled", "reel_us": 5000000,
     "cue": {"earliest_us": 4500000, "latest_us": 4600000, "source": "visual"}},
    {"cast_us": 8000000, "outcome": "missed",
     "cue": {"earliest_us": 12000000, "latest_us": 12100000, "source": "audio"}}
  ]
}
```

Outcomes are `reeled`, `missed`, `no_bite` and `uncertain`. Only a reeled row takes `reel_us`. An independently verified cue may use `visual`, `audio`, or `simulator` and reports a response interval. Leave the cue out when its onset is unknown. Record device, sensitivity and client size alongside new sessions so you can interpret changes in timing or movement.

Pass `-timing-train name1,name2 -timing-model-out <new-file>` to freeze a model. The names are recording directory basenames. The command requires at least one held-out session and a reviewed mouse hold from a complete training trace. It keeps reel hold and cooldown paired where the next cast was also reviewed in the same uninterrupted span. Hold samples must be within 10–500 ms and cooldowns within 250 ms–60 s. The model records trace and review hashes for both splits. A missing cooldown uses the app's configured value. Cue intervals are reported for analysis.

Run the app with `-dry-run -timing-model <file> -timing-seed 1 -trace-out <new-jsonl>` to sample the frozen profile. For a fixed comparison, use the same scenario and omit `-timing-model`. The fixed profile uses the 300 ms reel delay, a 30 ms simulated right-button hold and the configured cooldown. Give the app and fake game the same `-trace-session` label within each trial and a different label for each trial.

Use `responsebench -compare -json <game-and-app-traces...>` to compare simulated holds, release-to-cast gaps, and detector-to-press times with reviewed human sessions. An aligned unmatched bite or reel is a timing association, and a simulated release describes the app's callback sequence. Review the cancellation reasons and check missed bites and unmatched reels before comparing timing distributions.

The aligned comparison counts bites with a matched simulated reel release separately from matched reel presses. The press must follow the cue within the match window and the release must be within the overlapping recording. These counts describe the dry-run callbacks.

`-input-timing` reports gameplay patterns over consecutive reviewed casts. `-compare` reports the same pattern statistics for a dry-run app trace. A complete cycle starts at a cast and ends at the next cast. The latest two cycles are compared against earlier nonoverlapping pairs from the last 16 cycles. A match requires the same reel counts and cycle lengths, cast-to-first-reel times, and last-reel-to-cast times within 250 ms. Focus or capture gaps break the sequence, as do uncertain or unreviewed casts in manual recordings. Fewer than four contiguous cycles leave the repeat fraction unscored.

For optional reaction timing, run independent fixed simulator trials using the same bite detector and pass their JSON comparison to `-input-timing -timing-calibration <comparison.json>` when exporting the model. The fitter needs five matched cues and uses their median cue-to-detector delay. For reviewed visual or simulator cues precise to 250 ms, the app subtracts that detector delay and time already spent since detection from the sampled response interval midpoint. The fixed delay applies to samples without a usable cue or when the detector changes.

Use the headless paired replay to check a new model against three scripted bites with the real engine and FSM. Supply the model's absolute path in `VRL_TIMING_REPLAY_MODEL` and run `go test ./domain/fishing -run TestPairedTimingReplay -count=1 -v`.

`-live-timing -timing-model <file>` explicitly enables the model's holds and cooldowns for live input. Live runs keep the fixed reel delay because the detector calibration is from the simulator. The input guard checks focus, window bounds and pointer position before pressing. Stop, focus loss, geometry changes and shutdown release a pending hold. The ESP32 uses the requested Interact hold with firmware that advertises `TAPMS`. Older firmware keeps its 40 ms hold. Live timing uses a fresh seed on launch unless `-timing-seed` is supplied.

## Recorded Cursor Movement

`responsebench -cursor-fit` uses reviewed reel press times from `timing-review.json` to find free cursor approaches within each cast. It splits movement at capture gaps, mouse buttons, wheel input and keyboard presses. An approach must cover at least 20 pixels and end near the reviewed click. A separate small movement toward that click can supply a settling correction.

```powershell
go run ./cmd/responsebench -cursor-fit -cursor-train 20261108-012132 -cursor-model-out responsebench-results/cursor-profile.json datasets/20261108-012132 datasets/20261108-035820
```

Only named training sessions supply paths to the model. For each held-out reach, the report compares a direct move and 12 seeded fitted paths against the same human start and target. It reports errors in approach duration, travel divided by straight-line distance, and peak sideways deviation as a fraction of that distance. Correction counts are separate. Repeated seeds measure variation from the model and share the same human reference. A small split is suitable for pilot comparisons.

The `reach_audits` section keeps each held-out session separate. It reports peak normalized speed, when that peak occurred, and mean step interval. For path repetition, it resamples each reach to 32 points on a normalized clock and compares it with up to 16 earlier reaches using band-limited dynamic time warping. A smaller nearest distance means a more similar preceding path. Each fitted seed runs through the entire session with its own planner. Direct jumps have zero duration and are counted separately. The fitted values describe planned waypoints without optional settling. These exploratory scores have no alert threshold.

The planner samples paths weighted by reach distance, rotates and scales their recorded waypoints, and clips them to the client area. It spaces the approach steps evenly at roughly 16 ms and gradually corrects for any difference between the recorded endpoint and the intended destination. On some monitoring approaches it pauses near the bobber and makes one final correction. The final waypoint is the exact reel target. A bite replaces any unfinished approach and waits for the new path to reach the target. Stop, focus loss, a changed client area or user takeover cancel pending waypoints. Additional gestures are limited to that observed pointer correction. The separate timing profile can also vary reel holds and recast waits.

Use `-dry-run -cursor-model <file> -trace-out <new-jsonl>` for simulated movement. `-live-cursor -cursor-model <file>` explicitly enables real pointer movement and requires the pointer to start inside the selected client area. Both can be combined with their respective timing-profile flags. `-timing-seed` controls both profiles in a dry run.

Set `VRL_CURSOR_REPLAY_MODEL` to the model's absolute path and run `go test ./domain/fishing -run TestPairedCursorReplay -count=1 -v` to compare direct and fitted paths using the real FSM and capture engine with scripted frames.
