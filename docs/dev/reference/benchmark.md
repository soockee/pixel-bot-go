---
title: "Response benchmark"
summary: "What responsebench measures, its outputs, and the trace formats it shares with the app."
order: 8
---

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


## ESP32 camera recovery replay

Run this test to print the camera commands, test casts and elapsed simulated
time for each recovery scenario.

```powershell
go test ./domain/fishing -run '^TestFakeGameESP32CameraRecovery$' -v -count=6
```

The replay uses rendered fake-game frames, the real marker and bite detectors,
and the real fishing state machine. It models a signed camera heading from
the ESP32 drag callbacks. In the recoverable scene, reversing the first drag
makes the next cast produce a bobber. In the obstructed scene, every test cast
fails and the run requests logout once. The clock is virtual and the replay
does not model Bluetooth latency or the game's camera sensitivity.
