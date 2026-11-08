---
title: "Reproduce the response benchmark"
summary: "Run responsebench, stream trace feedback and read the app's dry-run alerts."
order: 7
---

Commit your changes first, so the manifest ties the run to one revision. Then run:

```bash
go run ./cmd/responsebench -out responsebench-results/my-run
```

The run takes a few seconds, and it refuses to write into an existing directory. Compare `CHECKSUMS.txt` with the reference run; identical lines mean identical bytes. To check saved files against their checksums, run `sha256sum -c CHECKSUMS.txt` inside the result directory.

To see how the frozen controllers were chosen, run `go run ./cmd/responsebench -split dev -out responsebench-results/dev`. Do not change `frozen` in `cmd/responsebench/config.json` after looking at eval results; `TestFrozenSettingsMatchDevSelection` fails if it no longer matches the dev selection. For a variation, copy the file, edit the copy, pass it with `-config`, and report that run as exploratory.

## Run streaming trace feedback

```bash
go run ./cmd/responsebench -trace-feedback -split dev -out responsebench-results/trace-feedback-dev
go run ./cmd/responsebench -trace-feedback -split eval -out responsebench-results/trace-feedback-eval
```

Use `-config` to supply scenario families and scorer settings, or `-seeds 200000,200001` to run a subset of the selected split. During development, choose settings using only the dev split, then freeze the config for evaluation. Read `summary.md` for the paired results and `metrics.jsonl` for each signal, its supporting event IDs, and any controller adjustment.

The default config also enables two independent detectors in this mode. Read `shadow-model.json` for fitted parameters and calibration cutoffs, `metrics.jsonl` for per-profile alerts, and `shadow-reference.jsonl` for false alarms on separate synthetic reference runs. Compare the backoff profiles in the paired results and check the `mixed` workload separately. That workload is excluded from fitting and calibration by default.

## Inspect app dry-run replay alerts

From the repository root in PowerShell:

```powershell
$env:RESPONSEBENCH_REPLAY_OUT = 'responsebench-results/app-dry-run-replay'
go test ./cmd/responsebench -run '^TestFakeGameReplayFeedsDryRunAttemptsToScorer$' -count=1 -v
Remove-Item Env:RESPONSEBENCH_REPLAY_OUT
```

Use a fresh output directory. The replay feeds rendered quiet and splash frames to the real fishing state machine and runs its cast and reel callbacks through the app's dry-run action interface. Both independent detectors observe that trace on Go's test clock. The test also runs as part of `go test ./...`; the environment variable enables file export.

Use `dry-run-alerts.jsonl` to find the alert time, detector score, cutoff, triggering record, and contributing cast IDs. Match those IDs to `event_id` in `dry-run-trace.jsonl`. The export also includes the detector summary and frozen model. A `dry_run_dispatched` outcome records callback execution only. This replay has no server replies or live retry outcomes.

## Poll dry-run alerts while the app runs

Use a frozen `shadow-model.json` from a responsebench or replay run. In PowerShell, start the app in one terminal and query the loopback endpoint from another:

```powershell
go run . -dry-run -trace-model responsebench-results/shadow-detectors-eval/shadow-model.json
```

```powershell
Invoke-RestMethod http://localhost:6060/debug/state | ConvertTo-Json -Depth 8
```

Select the local simulator in the app and start capture. The response includes the usual FSM fields and a `trace_observer` object with both detectors' totals and the last 32 alerts. Each alert has its elapsed time, score, cutoff, triggering record, and cast IDs. The model flag enables debug for that process and requires dry-run.
