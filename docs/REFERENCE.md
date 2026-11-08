# Reference

Central factual lookups: algorithms, config parameters, state/event tables, profiling endpoints.

---
## NCC Formula
The normalized cross-correlation score used in template matching:
$$
	ext{score} = \frac{\sum_i F_i T_i - n\,\mu_F\,\mu_T}{n\,\sigma_F\,\sigma_T}
$$
Where:
* $F_i$ – frame pixel value at position $i$ (after any grayscale or channel selection)
* $T_i$ – template pixel value at position $i$
* $n$ – number of contributing pixels
* $\mu_F, \mu_T$ – means of frame patch and template
* $\sigma_F, \sigma_T$ – standard deviations

Normalization removes brightness bias; higher score means better similarity (range roughly -1..+1; negative/inverted ignored here).

---
## What Counts as the Bobber
The bobber is whatever the addon marks with its four corner brackets, so other players' bobbers are ignored. `detect.MarkerLocator` looks for these in the capture:

| Property | Value |
| -------- | ----- |
| Colour | About RGB 255, 178, 163, within 28 per channel |
| Bracket | Two arms of at least 10 px from one outer corner, 3 px thick and 19 px long in the addon's default look |
| Box | 30 to 400 px per side, with at least 3 of 4 corners visible |

The box centre is the click point and the box is the area watched for a bite. The whole box must fit inside the selection. If the addon is off or its colour changes, nothing is found and the run halts after `max_failed_casts`. `roi_size_px` and the matching settings apply only to `detecteval -locator ncc`.

## Template Matching (Evaluation Only)
Locate template (small) within frame (large) returning (x,y,score,scale,found?). Score threshold gates action.

## Performance Techniques
1. Integral images for O(1) sums/variance.
2. Stride coarse pass + optional refine.
3. Parallel scale workers; every scale runs, so a frame always gives the same result.
4. Brightness normalized. The template is fully opaque; alpha is not used as a mask.

## Algorithm Flow
```text
generate scale list
for each scale in parallel:
    use the template prepared for that scale
    run NCC with (stride, threshold)
    send best score for that scale
collect results
pick overall best
if refine enabled and stride > 1:
    locally re-scan around best with stride=1
return (x,y,score,scale,found?)
```

## Config Parameters
| Setting                     | Meaning                                     | Tradeoff                               |
| --------------------------- | ------------------------------------------- | -------------------------------------- |
| Threshold                   | Minimum NCC score considered a hit          | ↑ fewer false positives, ↓ sensitivity |
| Stride                      | Pixel step while scanning, capped at a quarter of the template short side | Above the cap it has no effect |
| Refine                      | Precise second pass around best coarse spot | Slight cost, better accuracy           |
| MinScale/MaxScale/ScaleStep | Scale search range                          | Wide + tiny step = heavier workload    |
| CastKey (`cast_key`)        | Key that casts: F1 to F12 or a letter       | An unknown key halts the run           |
| StopKey (`stop_key`)        | Global hotkey that halts the run            | Hidden from other apps; read at start  |
| MaxFailedCasts              | Casts in a row without a reel before halting | Default 5                             |
| MaxSessionMinutes           | Run length; halts at the next cast after it | Default 60                             |
| Locator (`locator`)         | Technique that finds the bobber             | `marker` by default; see `detect.LocatorNames` |
| BiteDetector (`bite_detector`) | Technique that recognises a bite        | `motion` by default, `splash`, or `splash_sound` (splash plus the bite sound; listens to the selected window's process) |
| RecordDir (`record_dir`)    | Folder for new recordings                   | Default `datasets`                     |
| RecordKeys (`record_keys`)  | Record keys as well as the mouse            | Default true                           |

## Capabilities
* Watch a screen region for the bobber template.
* Multi-scale template matching with stride + refine pass.
* Response state machine (cast → search → monitor → reel → cooldown).
* Bite detection via grayscale ROI motion heuristics (`motion`), or the splash brightening the centre of the marker box (`splash`).
* Dark mode UI and adjustable selection area.
* JSON config persistence and structured event logging.

## Command-Line Flags
| Flag / variable                 | Effect                                                                                      |
| ------------------------------- | ------------------------------------------------------------------------------------------- |
| `-dry-run` / `PIXELBOT_DRY_RUN=1` | Log key presses, cursor moves and clicks as `dry-run action` instead of sending them; no Administrator needed |
| `-screen N` / config `screen`   | Open windows and the selection overlay on monitor N (1 = primary, others left to right); 0 = default placement |
| `-list-screens`                 | Print the monitors as JSON (`screen`, `x`, `y`, `w`, `h`, `primary`) and exit               |
| `-record <dir>`                 | Record the saved selection into a new directory at about 10 fps until Ctrl+C; opens no app window and sends no input |
| `-window "<title>"`             | With `-record`: also record your mouse and key input in that window to `input.jsonl`, the sound of its process to `audio.wav`, and serve the dashboard |
| `-no-keys`                      | With `-record`: record only mouse input |
| `-listen <addr>`                | Dashboard address, loopback only (default `127.0.0.1:6070`); empty disables it |
| `-view <path>`                  | Open the dashboard for a saved session directory or trace file |
| `-list-windows`                 | Print visible window titles and exit |
| `-auto-window "<title>"`        | Preselect the target window title (must match the window title exactly, case-insensitive)   |
| `-auto-start`                   | Start capture 0.5 s after launch; refused unless `selection_x/y/w/h` are saved in the config |

With `"debug": true` in the config, `http://localhost:6060/debug/state` returns the live FSM state as JSON, and `detection.search` entries (found, score, scale, position; misses sampled 1 in 10) are written to the log. `go run ./cmd/fakegame` opens a scripted simulator scene for local testing (`-scenario file.json`, `-pos x,y`, `-title`).

## Generated Files
| File                    | Purpose                                  | Notes                                                   |
| ----------------------- | ---------------------------------------- | ------------------------------------------------------- |
| `pixle_bot_config.json` | Persist user settings between runs       | Safe to edit while app closed; delete to reset defaults |
| `pixel_bot_logs.json`   | Structured log of events & state changes | Can be deleted; recreated automatically                 |

## Response Benchmark
`cmd/responsebench` compares two pacing and retry policies on seeded synthetic workloads with a virtual clock. It does not use the desktop app, its state machine or OS input. All values are synthetic and are not measured from a real system.

Flags: `-out` (new result directory, default `responsebench-results/eval`), `-split` (`eval` runs the frozen pair, `dev` ranks all candidates), `-config` (experiment file, default the embedded `cmd/responsebench/config.json`).

| File | Content |
| ---- | ------- |
| `config.json` | The exact experiment definition used |
| `scenarios.jsonl` | Every generated scenario: timeline, fault windows, environment |
| `traces.jsonl` | Eval only. Raw records: `scenario_id`, `seed`, `controller`, `t_ms`, `event_id`, `action` (`arrive`, `attempt`, `reply`, `close`), `attempt`, `outcome` |
| `metrics.jsonl` | One row per controller and scenario, computed only from the trace |
| `summary.json`, `summary.md` | Paired differences with 95% bootstrap intervals, or the dev ranking, plus metric definitions |
| `manifest.json` | Revision, uncommitted files, Go version, platform, config hash |
| `CHECKSUMS.txt` | `sha256sum` format, written last; excludes the manifest so machines can be compared |

Model: one action runs at a time, for `service_ms`, or `attempt_timeout_ms` if it starts in a hang window. An action that starts in a fail window fails after `service_ms`. Fault windows depend only on virtual time, so both controllers face the same environment. Fixed-interval acts on tick boundaries and retries until the event closes, rotating failed events to the back of its queue. Bounded backoff waits a seeded delay in `[min_delay_ms, max_delay_ms]` before a first action and between actions, and retries after a capped exponential backoff, up to `max_attempts`. Neither controller sees deadlines or future events.

Selection rule, applied to the dev split only: stay within `max_actions_per_event`, then highest mean completion at 0.1% resolution, then lowest mean censored p95 latency, then fewest actions, then name.

## FAQ
| Question                               | Answer                                                                    |
| -------------------------------------- | ------------------------------------------------------------------------- |
| Do I need to tweak settings first run? | No, defaults usually work.                                                |
| Why run as Administrator?              | Required for reliable mouse movement and key injection on Windows.        |
| It misses bites or reels late          | Lower `Threshold` slightly or reduce `Stride`; ensure refine enabled.     |
| False positives on water ripples       | Raise `Threshold` or narrow selection area.                               |
| Slow over time                         | Reapply settings or reduce scale range; profile for hotspots.             |
| How to reset everything?               | Close app, delete `pixle_bot_config.json` (and optionally logs), restart. |

## State Machine: States
| State         | Meaning                 | Exit Condition                              |
| ------------- | ----------------------- | ------------------------------------------- |
| halt          | Idle                    | Enable capture → waiting_focus              |
| waiting_focus | Await game window focus | Focus → searching                           |
| searching     | Scan template scales    | Found → monitoring; timeout/force → casting |
| monitoring    | Watch bobber motion     | Bite → reeling; lost or timeout → casting   |
| reeling       | Move, click after 300 ms | Click → cooldown                           |
| cooldown      | Delay before next cast  | Timer → casting                             |
| casting       | Send cast key           | Immediate internal → searching              |

## State Machine: Events
| Event                    | Source               | Effect                          |
| ------------------------ | -------------------- | ------------------------------- |
| EventAwaitFocus          | User enables capture | halt → waiting_focus            |
| EventFocusAcquired       | Focus watcher        | waiting_focus → searching       |
| EventTargetAcquiredAt    | Engine               | searching → monitoring          |
| EventTargetLost          | Tests                | monitoring → casting            |
| EventFishBite            | Engine               | monitoring → reeling            |
| ForceCast                | Tests                | searching, monitoring, cooldown → casting |
| Deadline                 | One state timer      | searching, monitoring → casting; reeling → click → cooldown; cooldown → casting |
| EventFocusLost           | Focus watcher        | any active state → waiting_focus |
| EventHalt                | Stop, hotkey, input guard | any → halt                 |

## Detection Datasets
`-record` writes `frames/<elapsed ms>.png`, and `input.jsonl` and `audio.wav` with `-window`. Frame names, trace times and audio count from the same start: sample n of `audio.wav` (16-bit PCM, stereo, 44.1 kHz) played n/44100 s after it, and silence fills the time the game played nothing. The sound is a copy of the game process and its child processes only (Windows process loopback, Windows 10 2004 or later); the game stays audible and no other program's sound is recorded. If the sound cannot be captured, the status says so and the recording continues without it. and `session.json` holds the offset from window positions to frame pixels. Coordinates in `labels.json` are pixels in the frames. Write it by hand or generate it with `detecteval -label`.

```json
{
  "casts": [
    {"start_ms": 1200, "end_ms": 15800, "bobber": {"x": 212, "y": 148, "w": 24, "h": 18}, "bite_ms": 12400},
    {"start_ms": 21000, "end_ms": 37000, "bobber": {"x": 240, "y": 160, "w": 24, "h": 18}}
  ],
  "empty": [{"start_ms": 16000, "end_ms": 20000}]
}
```

A cast covers the time the bobber is on screen. Leave out `bite_ms` if no bite came. `empty` lists spans with no bobber.

`go run ./cmd/detecteval [-config file] [-locator names] [-bite names] [-json] dir...` compares techniques across all the datasets given and prints one column per technique. `-locator` and `-bite` take comma-separated names, and both default to `all`:

| Metric | Meaning |
| ------ | ------- |
| Bobber found per cast | Casts where any frame gave a match |
| False locks on empty frames | Matches in `empty` spans, per frame |
| Click point inside the bobber | First matches of a cast inside the labelled rectangle, per lock |
| Bites caught | First trigger from 200 ms before `bite_ms` to 1500 ms after it |
| False bites per cast | First trigger earlier than that, or in a cast with no bite |
| Bite latency p95 | Trigger time after `bite_ms`, rounded up to 0 for early triggers |

A detector that listens, such as `splash_sound`, also gets the session's `audio.wav` in play order, and datasets without sound are skipped for it. Bite metrics watch the marker box from the frame it locks on, landing included, as the app does, and otherwise a square around the labelled bobber from the cast's start. Every bite detector sees the same areas, and the choice of locator does not affect them. `n/a` means nothing was measured. All values except search time repeat exactly. Recordings run at about 10 fps and the live app at 30 fps, so bite numbers are an approximation. `bite_ms` from `-label` is the reel minus a fixed reaction time, while real reactions vary from 0.8 to 1.7 s, so a trigger on the real splash can count as a false bite.

`detecteval -label <dir> [-cast mouse:x1] [-reel mouse:right] [-reaction 1200] [-box 40]` pairs each cast with the next reel click. Bindings are `mouse:<button>` for a click or `key:<name>`, including chords such as `key:Shift+B`.

| Label | Value |
| ----- | ----- |
| Cast span | From 2 s after the cast, once the bobber has landed, to the reel click |
| `bobber` | A `-box` px square around the reel click |
| `bite_ms` | The reel click minus `-reaction` |
| `empty` | From 1 s after a reel to the next cast, if at least 1 s long |

A cast with no reel, a reel within the first 2 s, and a click outside the frame are skipped and listed. Every labelled cast has a bite, so false bites can only count against early triggers.

`detecteval -export <new dir> dir...` saves each cast's watched area, from the cast until the reel, as `<dataset>/cast-NNN/<ms>.png`. `index.jsonl` has one row per crop with `file`, `dataset`, `cast`, `ms`, `box`, `reel_ms` and the estimated `bite_ms`.

## Shared Playtest Trace Interface

All four producers can emit UTF-8 JSONL with one `session_start` header, ordered event rows, and a `session_end` footer. The header identifies `format: "fishing-trace"`, `schema: 3`, source, clock, and optional `session_id`. An event carries `type` and elapsed microseconds in `t_us`. An `event_id` correlates events inside their producer and run, including ID zero. IDs from separate processes have independent meanings.

| Source | Clock | Events |
|---|---|---|
| `manual` | Monotonic with UTC start | Mouse and key transitions, click and drag summaries, scope changes |
| `fake_game` | Monotonic with UTC start | Displayed phase changes and `bite_available` |
| `app_dry_run` | Monotonic with UTC start | `cast_requested`, `cast_dispatched`, `bite_detected`, `reel_dispatched` |
| `responsebench` | Virtual, resets per `run_id` | `bite_available`, `reel_attempt`, `reel_reply`, `bite_closed` with scenario, controller, and seed |

A dry-run dispatch represents callback execution. Simulated completion comes from a benchmark close event. Manual inputs become cast or reel candidates only when the comparison is given an explicit binding. A mapped mouse input uses the press time reconstructed from a completed click and its duration. Camera drags and key repeats stay out of those mapped counts. A missing footer is shown as an unfinished trace, and malformed events stop the comparison.

Reel spacing uses adjacent actions in the same run and resets at recording gaps. Internal bite-to-reel latency uses the first matched action by event ID. Its start is bite availability in the benchmark and the app's detected bite in desktop dry-run. Reports show p50 and p95 in milliseconds or microseconds depending on output format.

To align a fake-game trace with app or manual input, give both live processes the same `-trace-session` label on the same machine. Comparison adds each producer's monotonic elapsed time to its UTC start. It pairs each displayed bite with the first unused reel candidate before the next bite and within `-match-window`, counting bites only in the overlapping time range. These are timing associations. The virtual benchmark clock stays separate. Recorder schemas 1 and 2 and previous benchmark and dry-run replay exports remain readable. Legacy traces lack the shared session label and are summarized independently.

### Manual activity profile

The comparison reports completed clicks and drags, ordinary key presses, shortcuts, wheel packets, and free cursor-movement bursts. A burst ends after a 250 ms gap, a button press, or an interruption in mouse capture. Movement during a button hold contributes to the gesture instead. Key repeats and standalone modifier keys stay out of the mix.

The activity-mix score is Shannon entropy over the six action counts, divided by the maximum entropy for six categories and scaled to 0–100. Zero means one category accounts for all counted actions. A higher value means activity is spread more evenly. This is a description of the recording. Compare sessions with similar tasks, devices, and keyboard capture settings. The score is available for manual traces with gesture labels. The capture-minute rate uses closed foreground segments of completed traces.

The profile also reports the drag fraction of completed mouse gestures, total raw travel across drags, and right-button drag count, hold time, and raw travel. Raw device counts depend on mouse settings and should be compared using the same device configuration.

## Streaming Trace Feedback

`-trace-feedback` compares the frozen fixed and backoff profiles with a third profile that uses the same backoff settings and random stream plus scorer feedback. It observes the fake-game timeline benchmark in simulation order. The observer sees each attempted reel before the next scheduling decision. It has no access to future events. `-seeds` accepts a comma-separated subset of the selected split.

The scorer uses the most recent `window_actions` simulated reel attempts. Interval regularity is the share of consecutive gap comparisons within `interval_tolerance_ms`. Sequence repetition is the share of adjacent two-action motifs that repeat and contain a retry. Retry burst strength is the count of retries inside `retry_window_ms` divided by `retry_burst_count`, capped at one. The composite takes the maximum feature. A signal fires on an upward crossing of `threshold`. Each signal records all three feature values and the relevant event IDs.

Interval and sequence signals can impose a short hold on future attempts. Retry signals can hold future retries while allowing first attempts. Holds use the configured `pacing_hold_ms` or `retry_hold_ms`, capped by the controller's maximum backoff. The benchmark models reel attempts and retry outcomes against splash arrivals. The desktop FSM and operating-system input stay outside this experiment.

`metrics.jsonl` includes the peak features, threshold crossings, and any applied holds. The flagged-run rate uses scorable runs, and first-signal latency uses flagged runs. Feedback begins after the first signal, so compare the two backoff profiles on total crossings and the fraction of scored windows above threshold. Completion and timeout rates still include every event.

### Independent detectors

When the config includes `shadow`, trace-feedback mode adds two silent observers. Neither feeds the controller. The **timing detector** estimates a discrete response hazard in 25 ms bins for first attempts, retries after failures, and retries after timeouts. Each context separates isolated and closely spaced bites. It learns from both completed waits and waits that end at a deadline, and averages the negative log likelihood of the last 12 observed waits. The **sequence detector** compares the latest four- and eight-reply patterns with earlier nonoverlapping patterns in the last 64 replies. Tokens include retry status, reply outcome, and a bucketed inter-attempt gap. It averages the stronger similarity across the last 12 comparisons.

Training and calibration use disjoint seeds and a synthetic reference policy with three response pacing bands and a separate retry schedule. The two backoff profiles and the fixed profile are excluded from fitting and calibration, as are workload families listed in `holdout_families`. Cutoffs use the rank of peak session scores on calibration traces and a strict greater-than comparison. The target applies to that pooled synthetic calibration set. The separate evaluation reference runs expose how false alarms change under different workloads.

Both observers use the same simulated-time grid for every controller on a scenario. Scores expire after `score_age_ms`. First-alert rates use scorable runs and time to first alert uses flagged runs. Time above threshold uses all ticks, including idle or unscored time, and is reported alongside scored-time coverage and repeated alert episodes. These detectors evaluate synthetic traces and their alerts have no live-game interpretation.

## Profiling Endpoints (pprof)
| Endpoint                        | Purpose            |
| ------------------------------- | ------------------ |
| /debug/pprof/                   | Index page         |
| /debug/pprof/profile?seconds=30 | CPU profile window |
| /debug/pprof/heap               | Heap allocations   |
| /debug/pprof/goroutine          | Goroutine stacks   |
| /debug/pprof/block              | Blocking profile   |
| /debug/pprof/mutex              | Mutex contention   |

## Common Fix Patterns
Reuse buffers; tune scale range; batch work instead of per-op goroutines; introduce backpressure via bounded channels.

## Glossary
| Term     | Definition                                     |
| -------- | ---------------------------------------------- |
| Template | Small target image to locate                   |
| Frame    | Full screenshot                                |
| Stride   | Pixel step size in scanning                    |
| NCC      | Normalized cross‑correlation similarity metric |
| ROI      | Region Of Interest after lock                  |
| FSM      | Finite State Machine for fishing loop          |
| RSS      | Resident Set Size (process working set)        |
| EMA      | Exponential Moving Average for bite detection  |

---
## Source Attribution
Content originally distributed across standalone detection, state machine, and debug guides now consolidated here. Former README details (capabilities, generated files, FAQ) also merged.
