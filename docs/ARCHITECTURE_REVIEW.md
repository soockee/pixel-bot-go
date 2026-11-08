# Architecture Review

Review of branch `feature/v1.1.0-prep` at commit `eb999ad` (2026-11-07), done before any new feature work. It lists every finding and the tasks that resolve them. The four Diataxis files describe the app as it is; this file is a work plan, so it stays separate and should be deleted or archived once the tasks are done.

Goals from the brief: human-readable code, less code, best-practice architecture, KISS and DRY.

---
## Summary

The app has a sound core idea: one event channel, injected input, a fake game for tests. Three things need fixing before features go on top of it.

1. **Input can go to the wrong place or at the wrong time.** Focus is only checked before the first lock. Stop does not reliably stop. The click lands on the corner of the match, not its centre.
2. **The detection pipeline lives in the UI layer.** It runs on the Tk tick inside a 369-line presenter, so it cannot be evaluated or swapped without the UI.
3. **Nothing measures detection.** There is no recorded footage from the real client, and the fake game draws the template pixel for pixel, so it cannot tell a good matcher from a bad one. Comparing algorithms, including ML, is guesswork until this exists.

There is also a lot to delete. The app has 5,053 lines of production code and 973 lines of tests. The tasks below should remove about 1,100 lines, roughly a fifth of the production code, before new tooling is added (estimate in [Target Architecture](#target-architecture)).

**Recommended order:** safety net, control fixes, structure, recorder and evaluation, better classical detection, and ML only if classical detection misses the targets.

---
## How to Read the Findings

| Priority | Meaning |
| -------- | ------- |
| P0 | Sends input to the wrong place or at the wrong time |
| P1 | Wrong results, or blocks the foundation or evaluation work |
| P2 | Cleanup: dead code, duplication, naming |

| Evidence | Meaning |
| -------- | ------- |
| Tested | Reproduced with a throwaway test against the real code (not committed) |
| Measured | Benchmarked on the dev machine (Core Ultra 7 265K, 20 threads) |
| Read | Found by reading the code; not reproduced at runtime |

---
## Findings: Controls

| ID | Pri | Evidence | Finding | Where |
| -- | --- | -------- | ------- | ----- |
| CTL-1 | P0 | Read | Focus is only checked in `waiting_focus`. After that, keys and right clicks go to whatever window is in front, including after Alt-Tab. | `ui/presenter/focus_watcher.go:44`, `domain/fishing/fsm.go:154` |
| CTL-2 | P0 | Tested | Stop can fail. `evtForceCast` is accepted in every state except `casting`. A timer event already in the channel when Stop runs casts from `halt` and returns to `searching`, whose 5 s timer then recasts forever. Start is ignored because the state is no longer `halt`. The timing window is narrow but the result sustains itself. Test: `ForceCast` from `halt` gave state `searching` and 2 key presses in 5.6 s. | `fsm.go:109` |
| CTL-3 | P0 | Tested | The reel runs in a goroutine with a 300 ms sleep that Stop cannot cancel. Test: bite, then Stop 50 ms later, still clicked once. | `fsm.go:165-178` |
| CTL-4 | P0 | Tested | The match position is the template's top-left corner, and it is used as the click target and the ROI centre. Test: bobber drawn at (200,120), reported (200,121). The click lands about 8,5 px from the centre of a 17x10 match. | `ui/presenter/detection_presenter.go:300-308` |
| CTL-5 | P0 | Tested | A late search result overwrites the target during `monitoring`. One more task can be queued when the lock happens, so this is reachable. The ROI then jumps, which looks like a bite. Test: coordinates changed to (50,50) while still `monitoring`. | `fsm.go:69` |
| CTL-6 | P1 | Read | If a selection capture fails, the loop falls back to a full primary-screen grab, but the presenter still adds the selection offset. Coordinates are then wrong. Clipped selections have the same problem. | `domain/capture/capture_service.go:115`, `detection_presenter.go:307` |
| CTL-7 | P1 | Read | `-auto-start` refuses to run without a selection; the Start button does not. Manual runs search the whole primary screen with a 170-pixel template and can click on a false match. | `app/app.go:133`, `ui/presenter/capture_presenter.go:48` |
| CTL-8 | P1 | Read | There is no way to stop from inside the game. Stop needs a click on the app window. | `ui/view/root_view.go:127` |
| CTL-9 | P1 | Read | Failed casts are never counted. If the game closes or the bobber is never found, the bot presses the cast key every 5 s without limit. | `fsm.go:248` |
| CTL-10 | P1 | Read | `keybd_event` and `mouse_event` are superseded by `SendInput`, return values are ignored, and the DLL is looked up on every call. | `domain/action/actions_windows.go:15-46` |
| CTL-11 | P1 | Read | `ParseVK` turns any unknown key into F3 without warning, so a typo presses the wrong key. It also has an empty branch. | `actions_windows.go:51-74` |
| CTL-12 | P2 | Read | `reel_key` is the cast key; the reel is a right click. | `config/config.go:40`, `fsm.go:154` |
| CTL-13 | P2 | Read | The 5 s search timeout and the 300 ms and 500 ms reel delays are hard-coded in the FSM. | `fsm.go:170,181,248` |

---
## Findings: FSM and Concurrency

| ID | Pri | Evidence | Finding | Where |
| -- | --- | -------- | ------- | ----- |
| FSM-1 | P1 | Read | Data races: timer callbacks read `f.state` and `f.closed`, `FSMPresenter.pending` is written by the FSM goroutine and read by Tk, and `FocusWatcher` shares `fired`, `lastTitle` and `done`. Double `close(w.done)` is possible. The race detector has never run. | `fsm.go:189,249`, `ui/presenter/fsm_presenter.go:36`, `focus_watcher.go:73` |
| FSM-2 | P1 | Tested | Sending after `Close` panics ("send on closed channel"), and the focus watcher can do this at exit. Double `Close` panics too. | `fsm.go:303` |
| FSM-3 | P1 | Read | A panic in the FSM loop is recovered, but nothing reads the channel afterwards. Public events use blocking sends, so Tk freezes after 64 events. | `fsm.go:46-53,277-285` |
| FSM-4 | P1 | Tested | Config does not reach the FSM's detector: it is built with `nil` and uses defaults, so `max_cast_duration_seconds` is ignored. `cooldown_seconds` is copied once at startup, so panel changes need a restart. | `fsm.go:39,232` |
| FSM-5 | P2 | Read | Three copies of the timer setup, `reeling` is rewritten to `cooldown` mid-function so it is never a real state, and `casting` recurses. | `fsm.go:154-262` |
| FSM-6 | P2 | Read | Dead API: `Tick`, `Cancel`, `EventTargetAcquired`, the `FishingStateListener` case. `EventFishBite` and `EventTargetLost` are used only by tests, while production takes a separate path through `evtMonitoringFrame`. | `fsm.go:64,113,280-289` |

---
## Findings: Architecture

| ID | Pri | Evidence | Finding | Where |
| -- | --- | -------- | ------- | ----- |
| ARC-1 | P1 | Read | The detection pipeline is in a UI presenter and runs on the Tk tick. A slow UI delays bite detection, and each ROI frame waits for a later tick, which adds at least 33 ms. It cannot run without Tk. | `ui/presenter/detection_presenter.go` |
| ARC-2 | P1 | Read | No algorithm seam. The presenter calls `DetectTemplateDetailed` directly, and `DetectorFactory` exists only to pass the config that FSM-4 drops. | `detection_presenter.go:287`, `domain/fishing/types.go:64` |
| ARC-3 | P1 | Read | Detection is spread across packages: NCC in `domain/capture`, bite logic in `domain/fishing`, ROI extraction in `ui/images`. | as listed |
| ARC-4 | P1 | Read | Frames do not carry their screen origin, so selection data is passed alongside them (`selection`, `hasSelection`, `SelectionRectProvider`). This is the root cause of CTL-6. | `domain/capture/metrics.go:9`, `detection_presenter.go:54` |
| ARC-5 | P1 | Read | Wiring is split between `BuildContainer` and `app.Run`, against the AGENTS.md rule. `AppContainer.Loop` is never set, and `width`, `height` are unused. | `app/container.go:39`, `app/app.go:89-112` |
| ARC-6 | P2 | Read | The FSM owns the bite detector and receives frames as events. Feeding the detector outside and sending `EventFishBite` makes the FSM states, timers and actions only. | `fsm.go:93-101` |
| ARC-7 | P2 | Read | Interfaces nobody needs: everything in `domain/capture/types.go`; the six sub-interfaces plus `FishingFSMContract`; and `CapturePreview`, `ConfigPanel`, `SessionStats`, `SelectionOverlay`, each with one implementation. Two of these are type-asserted back to the concrete type. | `domain/fishing/types.go:66-97`, `ui/view/root_view.go:159,349,430` |
| ARC-8 | P2 | Read | The config filename is written three times, and paths depend on the working directory. | `main.go:18`, `app/app.go:66-67` |
| ARC-9 | P2 | Read | `Validate` falls back to different values than `DefaultConfig`, for example threshold 0.80 vs 0.73, stride 4 vs 6, stop score 0.95 vs 0.80, min scale 0.60 vs 0.90. | `config/config.go:62-110` |

---
## Findings: Detection

| ID | Pri | Evidence | Finding | Where |
| -- | --- | -------- | ------- | ----- |
| DET-1 | P1 | Read | No real footage exists and the fake game uses the template itself, so accuracy on the real client is unknown and every threshold is unproven. | `internal/fakegame` |
| DET-2 | P1 | Measured | The template is only 17x10 px, 170 opaque pixels. That is very little signal, which explains the known false match. True match on the fake scene: 0.839. Wallpaper false match: about 0.77. Threshold: 0.73. | `assets/fishing_target.png` |
| DET-3 | P1 | Read | Matching is grayscale only, so the strong colours in the template are thrown away. | `domain/capture/ncc.go:337` |
| DET-4 | P1 | Tested | `analysis_scale` shrinks the frame but not the template, so it breaks matching. Results below. | `detection_presenter.go:276-287` |
| DET-5 | P1 | Measured | Search is slow and allocation-heavy: per-pixel `At()`, three float64 tables per frame, `i/w` and `i%w` per pixel. Results below. | `ncc.go:247-272,337` |
| DET-6 | P1 | Tested | The bite baseline barely moves. `int(diff * 0.03)` truncates to 0 for any difference under 34, so the EMA stays frozen. Test: brightness drifted 100 to 130, EMA stayed at 100. After that drift, a small flicker on 15% of the ROI counted as a bite; without drift, it did not. | `domain/fishing/bite_detector.go:224` |
| DET-7 | P1 | Read | One frame is enough to trigger: `frameDebounceNeeded` is 1, there is no settle time after lock, and the cursor moves onto the bobber as monitoring starts. Any mouse-over highlight may land in the first frames. Needs real footage to confirm. | `bite_detector.go:53,203`, `fsm.go:220` |
| DET-8 | P1 | Read | The global template cache is keyed by size only, so two templates of the same size share an entry. A second template would silently use the first one's data. | `ncc.go:33` |
| DET-9 | P1 | Read | Early stop depends on which goroutine finishes first, so the same frame can give different results. Evaluation needs repeatable output. | `domain/capture/multi_scale.go:103` |
| DET-10 | P1 | Read | Bite timing mixes clocks: `Reset` and the timeout use `time.Now()`, `FeedFrame` receives a time and ignores it, and the frame's `CapturedAt` is never used. Replays cannot control the timeout. | `bite_detector.go:67,251`, `detection_presenter.go:358` |
| DET-11 | P2 | Read | The `stride` setting has no effect above 2, because it is capped at a quarter of the template's short side. | `ncc.go:244` |
| DET-12 | P2 | Read | Transparency claims in the docs are wrong in two ways: the template has no transparent pixels, and the code would treat them as black rather than masking them. | `ncc.go:60-68`, `docs/REFERENCE.md` |
| DET-13 | P2 | Read | Duplication and dead code in matching: the scan loop exists twice, the flat-template branch is unreachable for this template, `MatchTemplateNCC` and `DetectTemplate`-style wrappers add layers, `ScaleSpec` wraps one float, `DebugTiming` is always on, and `min`/`max` shadow Go builtins. | `ncc.go:200-310,393-427`, `multi_scale.go` |
| DET-14 | P2 | Read | 20 of the 33 `BiteDetector` fields do nothing useful: statistics that are written and never read, plus debounce state that a debounce of 1 cannot use. | `bite_detector.go:14-41,163-190` |

`analysis_scale` test (DET-4), fake scene, threshold 0.73:

| analysis_scale | Found | Score |
| -------------- | ----- | ----- |
| 1.0 | yes | 0.839 |
| 0.8 | yes, barely | 0.745 |
| 0.5 | no | 0.521 |

Search cost (DET-5), default config, one call:

| Frame | Wall time, all cores | Allocated |
| ----- | -------------------- | --------- |
| 560x420 | 25 ms | 5.7 MB |
| 1280x720 | 100 ms | 22 MB |
| 1920x1080 | 217 ms | 50 MB |

---
## Findings: Capture

| ID | Pri | Evidence | Finding | Where |
| -- | --- | -------- | ------- | ----- |
| CAP-1 | P1 | Measured | The capture loop has no rate limit, while the app uses at most about 30 frames per second. A grab takes 10 ms at 560x420 and 21 ms at 1920x1080, with 0.9 MB and 8.3 MB allocated each time. That works out at roughly 100 fps and 95 MB/s, or 48 fps and 400 MB/s. | `domain/capture/capture_service.go:97-143` |
| CAP-2 | P1 | Read | Quick Stop then Start can leave two capture loops running, because `Start` only checks a flag. | `capture_service.go:82-96` |
| CAP-3 | P2 | Read | Win32 cleanup: `GetLastError` is called through a separate proc, which is unreliable in Go; a `[1 << 30]byte` cast where `unsafe.Slice` fits; `syscall.NewLazyDLL` here and `windows.NewLazySystemDLL` elsewhere. | `domain/capture/capture_windows.go` |
| CAP-4 | P2 | Read | The repo never sets DPI awareness. On a scaled monitor, capture and cursor coordinates may disagree. Not checked inside the Tk runtime or on a scaled display. | `main.go` |
| CAP-5 | P2 | Read | The selection is a fixed screen rectangle, so it goes stale when the game window moves. | `ui/view/selection_overlay.go` |
| CAP-6 | P2 | Read | Whether `BitBlt` sees the real client in exclusive fullscreen is unverified. Decide on another capture API only after the recorder (T4.1) shows a problem. | `capture_windows.go` |

---
## Findings: UI

| ID | Pri | Evidence | Finding | Where |
| -- | --- | -------- | ------- | ----- |
| UI-1 | P1 | Read | The window dropdown shows the first title, but nothing is selected until the user changes it. A user who accepts the default waits in `waiting_focus` forever. The list also never refreshes. | `ui/view/root_view.go:115` |
| UI-2 | P1 | Read | The config panel shows ROI Size, but Apply ignores it. Invalid input is dropped with no message. | `ui/view/config_panel.go:66,135-140` |
| UI-3 | P1 | Read | Opening the config panel during capture creates a new, editable panel, because the locked state applied only to the old one. | `root_view.go:255` |
| UI-4 | P2 | Measured | The preview scales and PNG-encodes every tick on the Tk thread: 6.6 ms, 8 MB and 360,000 allocations per frame before Tk decodes it. | `ui/view/capture_preview.go:53`, `ui/images/scale.go:58` |
| UI-5 | P2 | Read | Noise: 66 `== nil` guards in production code, including 22 logger checks, for values the constructors already guarantee. `applyPalette` is 70 lines of them. | across `ui/`, `domain/` |
| UI-6 | P2 | Read | Dead code: `DetectionModel` (written, never read), `NewCapturePreview`, the `app` forwarding methods and stale comments, `goWg`, `FocusWatcher.reset`, `AvgCaptureMicros`, and two commented-out logger calls with their 109 lines in `debug/`. | `ui/model/detection_model.go`, `app/app.go:199-226`, `debug/` |
| UI-7 | P2 | Read | Small duplication: two geometry regexes, and the button label "Toggle Capture" does not show the current state. | `root_view.go:366`, `selection_overlay.go:146` |

---
## Findings: Testing, Security and Repository

| ID | Pri | Evidence | Finding | Where |
| -- | --- | -------- | ------- | ----- |
| TST-1 | P1 | Read | Releases build without running tests or vet, and the release job cannot test Windows code on Ubuntu. | `.github/workflows/auto-release.yml` |
| TST-2 | P1 | Read | The race detector has never run, and this machine has no C toolchain. CI on a Windows runner can cover it. | handoff notes |
| TST-3 | P1 | Measured | Coverage gaps: `config` and `app` have 0%, `ui/presenter` 25%. `domain/capture` also shows 0% because its only tests live in `internal/fakegame`. There are no direct NCC tests. | `go test -cover ./...` |
| TST-4 | P2 | Measured | FSM tests sleep on real timers and take about 8 s. `testing/synctest` makes them instant and repeatable. | `domain/fishing/*_test.go` |
| SEC-1 | P1 | Read | README and docs say Administrator is required, but injecting into a non-elevated game should work without it. A bot with a debug server should not run elevated without a reason. Needs one real test. | `README.md`, `docs/HOW_TO.md`, `docs/REFERENCE.md`, `AGENTS.md` |
| SEC-2 | P2 | Read | Release actions are pinned by moving tags in a workflow with `contents: write`. | `auto-release.yml` |
| SEC-3 | P2 | Read | `.github/chatmodes/gpt-5-beast-mode.chatmode.md` is unrelated to this project. | `.github/chatmodes/` |
| DOC-1 | P2 | Read | Docs have drifted: "low motion triggers EventTargetLost" (it is a timeout), `ForceCast` from any state, stride and transparency claims, and the HOW_TO steps for the debug loggers. | `docs/EXPLANATION.md`, `docs/REFERENCE.md`, `docs/HOW_TO.md` |

---
## Target Architecture

One pipeline goroutine replaces the presenter's worker, queues and tick dependency. Because search runs synchronously in that goroutine, a late result cannot exist, so CTL-5 goes away by design.

```text
fishing.Engine       one goroutine at 30 Hz, no UI imports
      |  capture.Source.Next(): a frame with its screen Origin
      |  searching:  locator.Locate(frame) -> fsm.EventTargetAcquiredAt(origin + centre)
      |  monitoring: bite.Feed(roi, frame time) -> fsm.EventFishBite()
      |  publishes the latest preview through an atomic pointer
      |
fishing.FSM          states, one deadline timer, input actions
      |
action (guarded)     foreground and bounds check, then SendInput

ui                   reads Engine and FSM state on the tick, renders preview at 5 to 10 fps
```

The seam for algorithms is two small interfaces, which are all the evaluation and the FSM need:

```go
package detect

// Locator finds the bobber. The point is its centre in frame coordinates.
type Locator interface {
	Locate(frame *image.RGBA) (Match, bool)
}

// BiteDetector watches the ROI around a locked bobber.
type BiteDetector interface {
	Feed(roi *image.RGBA, t time.Time) bool
	Reset()
}
```

The FSM uses a single deadline, because every timeout leads to the same place:

```go
func (f *FSM) arm(d time.Duration) {
	f.timer.Stop()
	f.gen++
	gen := f.gen
	f.timer = time.AfterFunc(d, func() { f.send(evtDeadline{gen}) })
}
// In the loop, evtDeadline with an old gen is ignored, so nothing stale can cast.
```

`reeling` becomes a real state: move the cursor on entry, then click when its deadline fires. Stop cancels that deadline, which fixes CTL-3 without a goroutine.

| Package | Role after the change |
| ------- | --------------------- |
| `app/` | Composition root; all wiring in one place |
| `config/` | One `DefaultConfig`, shared by `Validate` |
| `domain/capture/` | Frames only |
| `domain/detect/` (new) | `Locator`, `BiteDetector`, NCC, ROI extraction |
| `domain/fishing/` | FSM and Engine |
| `domain/action/` | Input, guards, hotkey, window and screen queries |
| `ui/` | Presents state; no detection logic |
| `cmd/detecteval/` (new) | Runs candidates over a recorded dataset |

Estimated size change (estimates, not measurements):

| Code | Now | After |
| ---- | --- | ----- |
| `detection_presenter.go` becomes Engine plus preview | 369 | ~120 |
| `ncc.go`, `multi_scale.go`, `detect.go` | 613 | ~230 |
| `bite_detector.go` | 261 | ~110 |
| `fsm.go`, `types.go` | 425 | ~240 |
| `debug/goroutines.go`, `debug/memstats.go` | 109 | 0 |
| `domain/capture/types.go`, `model/detection_model.go` | 65 | 0 |

---
## Detection Evaluation Plan

Measure first, then choose. Every candidate runs through the same harness on the same recorded data.

### Dataset
Record real sessions while fishing by hand, so no input is sent. Cover at least 3 zones, day and night, and 2 camera distances, with about 50 casts per condition. Add frames without a bobber, such as empty water and the desktop, to measure false locks. Keep the dataset out of git, per the repository rules in AGENTS.md.

### Proposed Targets
Starting points; adjust them after the first baseline.

| Metric | Target |
| ------ | ------ |
| Bobber found per cast | at least 98% |
| False locks on frames without a bobber | at most 1% |
| Click point inside the bobber | at least 99% of locks |
| Search time at the selection size | at most 100 ms |
| Bites caught | at least 98% |
| False bites per cast | at most 2% |
| Bite latency, p95 | at most 300 ms |

### Locator Candidates

| ID | Approach | Extra code | Why consider it | Risk |
| -- | -------- | ---------- | --------------- | ---- |
| L0 | Current NCC, fixed | none | Baseline | Weak 17x10 template |
| L1 | Larger templates cut from real footage, with a colour gate before NCC | small | Cheap, uses the discarded colour, much faster | Needs templates per zone or lighting |
| L2 | Motion: compare before the cast with after landing | ~30 lines | No template, independent of art and zoom | Waves, other players, moving camera |
| L3 | Tiny CNN scoring candidate patches from L1 or L2 | ~150 lines plus training | Handles lighting and zones | Needs labelled data |
| L4 | Full-frame object detector through ONNX Runtime | large | Best possible recall | See below; last resort |

A plain "classify the whole screen" model is not enough for search, because the click needs coordinates. Patch scoring (L3) or a grid or heatmap output provides them. Whole-image classification does fit bite detection, since the ROI is already known.

### Bite Candidates

| ID | Approach | Extra code | Why consider it | Risk |
| -- | -------- | ---------- | --------------- | ---- |
| B0 | Current heuristic, fixed | none | Baseline | Many hand-set constants |
| B1 | Track the bobber in the ROI with NCC; a bite is a sharp dip | small | Reuses NCC, measures the actual event | Needs a stable template |
| B2 | Splash sound through Windows audio | medium, COM | Independent of visuals | Needs game sound; other sounds interfere |
| B3 | Tiny CNN on a short ROI sequence | ~150 lines plus training | Learns the real signal | Needs labelled bites |

### ML Constraints
The whole stack is pure Go, and the release cross-compiles from Ubuntu. Cgo-based ONNX bindings would break that and add a bundled DLL. The better fit is to train in PyTorch outside the repo, embed the weights with `go:embed`, and run them with a small pure-Go inference function (convolution, ReLU, pooling, dense). A 32x32 model with three convolution layers is small enough for CPU, and it does not compete with the game for the GPU.

### Recommendation
Do L0 and B0 fixes first, because a buggy baseline would make every comparison wrong. Then try L1, L2 and B1, which cost little. Start ML only if the best classical result misses the targets. If it does, L3 and B3 are the right size. L4 should need a measured reason.

---
## Proposed Standalone Response Benchmark

This experiment supports a scientific paper and an internal engineering review. The research question is how pacing and retry policies affect response latency, completion, and action frequency under controlled workloads.

- Build a headless `cmd/responsebench` that consumes seeded scenario definitions and runs a virtual clock. Use the scenario model from `internal/fakegame` where it fits. Give the benchmark its own controller interface that takes scenario events and returns simulated actions. Both experimental controllers belong to the benchmark. The desktop application, its state machine, and its operating-system input path stay outside the harness.
- Compare a fixed-interval controller with a controller that uses bounded delays and retry backoff. Use the same scenario and seed pairs for both. Include sparse and clustered events, transient failures, and timeouts. Freeze controller settings after development runs, then evaluate on a separate set of scenarios.
- Save raw JSONL traces and per-run metrics. Record scenario ID, seed, controller, elapsed simulation time, event ID, action, and outcome. Report completion and timeout rates, missed events, median and p95 response latency, and retry counts. Keep failed runs in the denominators and state how latency is calculated when an event gets no response.
- Provide one command that generates paired results and records the scenario inputs, controller settings, and revision. Test virtual-clock ordering, event-to-action correlation, reproducibility, and timeout accounting. Preserve the raw runs and report the paired differences and uncertainty for the paper.

---

## Tasks

Each phase leaves the app working. Finding IDs show what a task resolves. Update tests and docs in the same change, per AGENTS.md.

### Phase 1: Safety Net
- [ ] **T1.1** Add a CI workflow on `windows-latest` for pull requests and `main`: `gofmt -l .`, `go vet ./...`, `go test ./...`, and `go test -race ./...` with `CGO_ENABLED=1`. Make the release depend on it. Pin actions by SHA. Resolves TST-1, TST-2, SEC-2. *Done when:* a failing test blocks a release. *Status:* `ci.yml` is written and wired into the release, but has not run on GitHub yet. The race step stays non-blocking until it has a clean run.
- [ ] **T1.2** Add regression tests for CTL-2, CTL-3, CTL-5, FSM-2, FSM-4 and DET-6, marked to skip until fixed. *Done when:* each skip is removed by the task that fixes it. *Status:* all six are in `domain/fishing/regression_test.go` and fail with `RUN_KNOWN_BUGS=1`. The skips go with their fixes.
- [x] **T1.3** Add `config` tests (defaults, clamps, load errors) and direct NCC tests in `domain/capture`. Resolves part of TST-3.
- [x] **T1.4** Remove `.github/chatmodes/`. Resolves SEC-3.

### Phase 2: Control Correctness
- [x] **T2.1** Rewrite the FSM around one deadline timer with a generation number. Make `reeling` a real state and run actions on the loop. Accept casts only from a live deadline. Resolves CTL-2, CTL-3, CTL-13, FSM-1 (FSM part), FSM-5. *Done when:* stale deadlines are ignored in a test, and a click cannot follow Stop.
- [x] **T2.2** Ignore target updates outside `searching`. Resolves CTL-5.
- [x] **T2.3** Replace `f.closed` with a `context`, make event sends non-blocking after shutdown, and make `Close` safe to call twice. A loop panic must halt the app visibly. Resolves FSM-2, FSM-3.
- [x] **T2.4** Pass a config copy to the FSM on Start. Resolves FSM-4.
- [x] **T2.5** Return the match centre (top-left plus half the scaled size) from matching. Update the fake-game tests. Resolves CTL-4.
- [x] **T2.6** Keep the focus watcher running in every active state. Add `EventFocusLost`, which goes to `waiting_focus` and cancels the deadline. Replace its fields with one goroutine and a context. Resolves CTL-1, FSM-1 (watcher part).
- [x] **T2.7** Add `action.Guarded`, built like `DryRun`, that drops input unless the selected window is in front and the target is inside the capture area. Resolves CTL-1 at the moment of input.
- [x] **T2.8** Add a global stop hotkey with `RegisterHotKey` on a locked OS thread. Config: `stop_key`. It calls `EventHalt` directly so it works even if Tk stalls. Resolves CTL-8.
- [x] **T2.9** Halt when the user takes over (cursor is not where the bot left it before a click), after `max_failed_casts` in a row (default 5), and after `max_session_minutes` (default 60). Resolves CTL-9.
- [x] **T2.10** Move to `SendInput`, check its result, load procs once. Make `ParseVK` return `(vk, ok)` and reject unknown keys in the config panel. Rename `reel_key` to `cast_key`. Resolves CTL-10, CTL-11, CTL-12.
- [x] **T2.11** Refuse Start without a saved selection, using the same check as `-auto-start`. Resolves CTL-7.
- [ ] **T2.12** Test without Administrator against the real client, then correct the docs. Resolves SEC-1.

Phase 2 status: T2.1 to T2.11 are done and covered by unit tests. A dry-run against the fake game caught 3 of 3 bites. Real keyboard, mouse and hotkey input was not exercised against a game. Dry-run skips `action.Guard`, so its checks are only covered by unit tests.

### Phase 3: Structure
- [x] **T3.1** Add `Origin` to `FrameSnapshot`. Remove the full-screen fallback when a selection is set, and remove selection plumbing from the pipeline. Resolves CTL-6, ARC-4.
- [x] **T3.2** Rate-limit the capture loop with a ticker at 30 Hz. Own the loop with a context and done channel. Resolves CAP-1, CAP-2.
- [x] **T3.3** Create `domain/detect`. Move NCC, the bite detector and ROI extraction there, behind `Locator` and `BiteDetector`. Resolves ARC-2, ARC-3.
- [x] **T3.4** Add `fishing.Engine` and shrink the presenter to preview only. Move the max-cast timeout into the FSM deadline, feed the detector from the Engine, and drop `evtMonitoringFrame` and `DetectorFactory`. Resolves ARC-1, ARC-6, DET-10. *Done when:* a replay test runs the whole pipeline without Tk.
- [x] **T3.5** Build all wiring in `BuildContainer`, with the view passed in. One constant for the config path. Resolves ARC-5, ARC-8.
- [x] **T3.6** Delete consumer-less interfaces and dead API, and use concrete types for single-implementation views. Resolves ARC-7, FSM-6, UI-6, and the commented code. Update HOW_TO.
- [x] **T3.7** Require a logger and dependencies in constructors, then remove the nil guards. Resolves UI-5.
- [x] **T3.8** Fix the UI: sync the dropdown default to the selection and add a refresh, apply ROI size, show invalid input, keep the panel locked during capture, show capture state on the button. Resolves UI-1, UI-2, UI-3, UI-7.
- [x] **T3.9** Render the preview at 5 to 10 fps and read `Pix` directly. Resolves UI-4.
- [x] **T3.10** Make one set of defaults the source for `Validate`. Resolves ARC-9.
- [ ] **T3.11** Win32 cleanup. Set per-monitor DPI awareness if a test on a scaled display shows it is needed. Resolves CAP-3, CAP-4. *Status:* the cleanup (CAP-3) is done. CAP-4 still needs a test on a scaled display.
- [x] **T3.12** Move FSM tests to `testing/synctest`. Add presenter and Engine tests. Resolves TST-3, TST-4.
- [x] **T3.13** Update AGENTS.md and the Diataxis docs. Resolves DOC-1, DET-12.

Phase 3 status: the Engine also took over the capture loop, so the capture service and its stats are gone. 

### Phase 4: Measure Detection
- [ ] **T4.1** Add `-record <dir>`: saves frames at about 10 fps with timestamps and sends no input. Confirms what capture sees on the real client. Resolves CAP-6. *Status:* `-record` works on the desktop. CAP-6 stays open until it records the real client.
- [ ] **T4.2** Record the dataset and create `labels.json` (bobber rectangle per cast, bite frame). Resolves DET-1. *Status:* needs hand play in the real client. `-record -window` and `detecteval -label` do the labelling; steps are in `docs/HOW_TO.md`. `fishing-recordings/manual-002.jsonl` (7 casts, 130 s) holds input events and no images, so it cannot be a dataset. It did show reels up to 25.4 s after a cast, which raised the `max_cast_duration_seconds` default from 16 to 30, and reel clicks inside a 77x71 px area, so a small selection is enough.
- [x] **T4.3** Add `cmd/detecteval`: runs a `Locator` or `BiteDetector` over a dataset and prints the target metrics. Unit-test it on a small generated dataset. *Done when:* two runs give identical numbers.
- [x] **T4.4** Make matching repeatable: remove early stop, key the template cache by template, and keep a per-`Locator` cache. Remove settings that stop meaning anything (`stop_on_score`, `return_best_even`, `stride` if unused). Resolves DET-8, DET-9, DET-11. *Status:* `stride` stays, because it is capped rather than unused; it will matter with larger templates (T5.4).
- [ ] **T4.5** Record the L0 and B0 baseline in this file. *Status:* first numbers from one 6-cast session. Locators: `marker` found 100% with 83.3% of clicks inside a 40 px label box, `ncc` found 83.3% with 0% inside. Bites with `motion`: 66.7% caught, 33.3% false, but its catches can include triggers from the cursor arriving, so this needs more casts with the cursor parked on the bobber. With bites watched from the lock, as the app does: `motion` 50% caught, 50% false, p95 1088 ms; `splash` 83.3% caught, 16.7% false, p95 535 ms, and it fires on the visible splash in all 6 casts: the one false trigger is the real splash 1.7 s before the reel, which the fixed 1200 ms reaction in the labels counts as early. A night session showed the marker's colour depends on the scene (brackets are partly transparent), so the locator now tests the colour's shape instead of one value. In that session the bite sound started about 250 ms after the visible splash. A 10-cast session with sound confirmed it: the bite sound follows the splash by 190 to 440 ms in every bite. Splash alone caught 4 and fired falsely in 4; sound alone heard every bite but also 2 other sounds while waiting; a splash followed by a sound within 0.8 s caught 9 of 10 with no false trigger, firing 140 to 620 ms before the player's click.

### Phase 5: Improve Classical Detection
- [x] **T5.1** Clean up NCC: one scan function, direct `Pix` access, reused buffers, no flat-template branch, no builtin shadowing. Add a benchmark. Resolves DET-5, DET-13. *Done when:* output is unchanged and allocations per search are near zero. *Status:* positions, matches and scores are unchanged on 240 baseline searches. A 1920x1080 search went from about 182 ms and 50 MB to about 50 ms and a few kB once buffers exist.
- [x] **T5.2** Fix or remove `analysis_scale`. Removing is the default, because a colour gate should give the speed-up. Resolves DET-4.
- [ ] **T5.3** Fix the bite detector: real EMA with float or fixed-point baseline, debounce and settle time chosen from data, and remove unused fields. Resolves DET-6, DET-7, DET-14.
- [ ] **T5.4** Build and evaluate L1 (larger templates, colour gate). Resolves DET-2, DET-3.
- [ ] **T5.5** Build and evaluate L2 (motion).
- [ ] **T5.6** Build and evaluate B1 (bobber tracking).
- [ ] **T5.8** Build and evaluate `splash`: the splash brightens the centre of the marker box, while other players' effects spread across it. *Status:* built (`domain/detect/splash.go`); thresholds come from one 6-cast session and need more footage before it becomes the default.
- [ ] **T5.9** Evaluate B2 (bite sound). Needs audio in recordings first.
- [ ] **T5.7** Ship the best candidates and delete the rest. Keep only code that won. Record results here.

The app now locates the bobber from the addon's corner brackets (`detect.MarkerLocator`), which replaces the template for the live locator and covers T5.4 and T5.5 unless the marker proves unreliable. Bite detection (T5.3, T5.6) still needs footage with the marker on.

### Phase 6: ML, Only If Targets Are Missed
- [ ] **T6.1** Export labelled patches and ROI sequences from the dataset. Keep training code outside the release build.
- [ ] **T6.2** Add pure-Go inference with embedded weights, with an output test against the training framework.
- [ ] **T6.3** Evaluate L3 and B3 through `cmd/detecteval`. Adopt only if they beat the classical result on the targets.

### Optional, Decide Later
- [ ] **T7.1** Derive the capture region from the selected window's client area instead of a fixed rectangle. Resolves CAP-5.
- [ ] **T7.2** Evaluate B2 (audio) if visual bite detection misses the targets in dark or busy scenes.

---
## Open Decisions

| Decision | Options | Recommendation |
| -------- | ------- | -------------- |
| When to release v1.1.0 | Before this work, or after Phase 3 | After Phase 2 at the earliest; the current build has the P0 issues |
| Where the dataset lives | Local folder, or a private release asset | Local to start |
| Fixed selection or window area | Keep the selection, or T7.1 | Keep it until the Engine lands |

---
## Review Limits

* Nothing was tested against the real game client. All accuracy statements come from the fake game or from the code.
* The race detector was not run, so FSM-1 comes from reading the code.
* `staticcheck` and `deadcode` were not run, because the tool download was not permitted here. Dead code comes from manual search and may be incomplete.
* Benchmarks come from one machine with few iterations. Treat them as rough sizes.
* The app was not launched, so Tk behaviour (UI-1, UI-3) comes from reading the code.
