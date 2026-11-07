# AGENTS.md

Visual Response Lab is a Windows Go application for visual event and response experiments. It captures a screen region, locates the target from the corner brackets an addon draws around it, and uses region-of-interest brightness statistics to trigger action callbacks. The local simulator supplies a scripted fishing scene for reproducible tests. See [README.md](README.md) and `docs/` (Diataxis: TUTORIALS, HOW_TO, REFERENCE, EXPLANATION).

## Stack

- Go 1.26. Module path is `github.com/soocke/pixel-bot-go` (one `e`), which differs from the GitHub owner `soockee`; use the `go.mod` spelling in imports.
- UI: `modernc.org/tk9.0` (pure-Go Tk). Win32 access via `golang.org/x/sys`.
- Capture and input injection (`domain/capture/capture_windows.go`, `domain/action/actions_windows.go`) are `_windows.go` files. Builds and runs on Windows only; the app needs Administrator to inject input.

## Commands

```bash
go build -o pixel-bot-go.exe .      # build
go test ./...                       # run all tests
go vet ./...                        # static checks
gofmt -l .                          # must print nothing
GOOS=windows GOARCH=amd64 go build ./...   # cross-compile check from non-Windows
```

There is no Makefile. `.github/workflows/ci.yml` runs gofmt, vet, tests and the race detector on Windows for pull requests, and `auto-release.yml` runs it before every release. The race step stays non-blocking until it has a clean run.

`domain/detect/bite_detector_test.go` holds failing tests for known bugs, skipped through `knownBug`. Set `RUN_KNOWN_BUGS=1` to run them, and delete the `knownBug` call in the change that fixes the bug.

## Layout

| Path | Role |
|------|------|
| `main.go`, `logger.go` | Entry point; loads `pixle_bot_config.json` (sic, the filename is misspelled on purpose, keep it), sets up slog, optional pprof on `localhost:6060` when `debug` is true |
| `app/` | `App` wrapper and `AppContainer` (`container.go`) that wires models, services, presenters and the root view |
| `config/` | `Config` struct, `DefaultConfig`, `Validate`, `Load` (JSON) |
| `domain/capture/` | `Source.Next` grabs one frame of the selected region; `Frame.Origin` is its screen position |
| `domain/detect/` | `Locator` and `BiteDetector` interfaces, `MarkerLocator` (the app's locator), NCC template matching (evaluation only), motion bite detector |
| `domain/fishing/` | `Engine` (`engine.go`, the capture and detection loop) and the FSM (`fsm.go`: states, deadline, input) |
| `domain/action/` | Key press, cursor move, right click (Windows input injection), `Guard` (safety checks on real input), global stop hotkey |
| `ui/model` `ui/presenter` `ui/view` | MVP-style UI: models hold state, presenters connect domain to views, views are Tk widgets |
| `ui/images`, `ui/theme` | PNG encoding for Tk, theming |
| `debug/` | `/debug/state` handler |
| `assets/` | Embedded `fishing_target.png` template (`embed.go`), used by the simulator and NCC evaluation |
| `cmd/detecteval/` | Compares every registered `Locator` and `BiteDetector` on recorded datasets, writes `labels.json` from a session with `-label`, and exports each cast's watched area with `-export`. See `docs/REFERENCE.md` |
| `internal/dataset/` | Dataset format, labels and the frame loop |
| `internal/recorder/` | `-record` sessions (frames plus mouse and key input in one window), the loopback dashboard and `-view` |
| `cmd/responsebench/` | Standalone virtual-clock benchmark of pacing and retry policies; run `go run ./cmd/responsebench`. See `docs/REFERENCE.md`. Output goes to the git-ignored `responsebench-results/` |

## Architecture notes

- FSM lifecycle: `waiting_focus → searching → monitoring → reeling → cooldown → casting → searching`. A single event channel serializes transitions; timers decouple cadence from UI ticks. Add new event sources by feeding that channel rather than mutating state directly. Each state has at most one deadline (`arm`), a transition cancels it, and input actions run on the loop, so nothing pending can act after Stop or a focus change.
- `fishing.Engine` runs the pipeline on one goroutine with no UI dependency: it grabs a frame, runs the detection the FSM state calls for, and sends the result to the FSM. The UI only reads `Engine.Preview` on its tick.
- Techniques are exchangeable: implement `detect.Locator` or `detect.BiteDetector` and add one line to `domain/detect/registry.go`. The `locator` and `bite_detector` config values pick them in the app and config panel, and `detecteval` compares them all by default.
- Algorithms plug in through `detect.Locator` and `detect.BiteDetector`, frames through `fishing.FrameSource`, and input through `fishing.ActionCallbacks`, so tests substitute fakes instead of touching Win32. Keep those seams when adding input, capture or detection behavior. Define other interfaces where they are consumed.
- The bobber is whatever the addon marks: `detect.MarkerLocator` returns the box inside its corner brackets, the box centre is the click point, and the box is the area watched for a bite. Without the addon the bot finds nothing and halts after `max_failed_casts`.
- Build all wiring in `app/container.go`; do not construct services inside presenters or views. Constructors require their dependencies, so do not add nil guards for them.

## Conventions

- Follow `.github/instructions/go.instructions.md` (idiomatic Go, early returns, documented exports, no emoji, English comments, exactly one `package` line per file when editing).
- Tests are table-friendly `_test.go` files beside the code (`fsm_test.go`, `engine_test.go`, `bite_detector_test.go`, presenter/model tests). Add or update tests for FSM, detector and presenter changes; run `go test ./...` before finishing.
- Windows-specific code goes in `*_windows.go` files.
- Config changes: update `Config`, `DefaultConfig`, `Validate`, and `docs/REFERENCE.md` together.
- Docs follow Diataxis; put new content in the matching file.

## Agentic testing and signals

- Logs: the app writes JSON slog entries to `pixel_bot_logs.json` (truncated each start). FSM transitions are logged at Info (`fishing state transition`, fields `from`/`to`), plus `cast action executed` and `reel action executed`.
- Live state: with `"debug": true` in `pixle_bot_config.json`, `http://localhost:6060/debug/state` returns JSON (`fsm_state`, `capture_running`, `selected_window`, `target`, `uptime_seconds`, `time`) next to the pprof endpoints. Implemented in `debug/state.go`, registered in `app.Run`.
- Replay tests: `domain/fishing/engine_test.go` runs rendered fake-game frames through the Engine, real matcher, real bite detector and FSM with recorded fake actions, without Tk. Extend it for new detection or FSM behavior. FSM and Engine tests use `testing/synctest`, so timers run on a fake clock.
- Fake game: `go run ./cmd/fakegame [-scenario file.json] [-title "Fake Game"]` opens a window with a noisy scene, the real bobber template with the addon's brackets and scripted bites (default loop: empty 2s, bobber 4s, splash 0.6s, empty 3s). Each phase change prints a JSON line on stdout (`t`, `phase`, `bobber` rect), to correlate with `pixel_bot_logs.json`. Scene logic lives in `internal/fakegame` and is unit-tested against the real template matcher and bite detector. Scenario JSON fields: `width`, `height`, `bobber_x`, `bobber_y`, `bobber_scale`, `seed`, `loop`, `splash_lum`, `timeline` (`[{"phase":"empty|bobber|splash","ms":N}]`).
- Dry run: `pixel-bot-go.exe -dry-run` (or `PIXELBOT_DRY_RUN=1`) logs key presses, cursor moves and clicks as `dry-run action` entries instead of injecting them; no Administrator needed. `Config.DryRun` is `json:"-"` so it is never saved. Implemented in `domain/action/dryrun.go`, wired in `app/container.go`.
- Headless run: `-auto-window "<title>" -auto-start` preselects the target window and starts capture 0.5 s after launch; the capture region comes from `selection_x/y/w/h` in `pixle_bot_config.json`. Search results are logged at Debug as `detection.search` (`found`, `score`, `scale`, `x`, `y`). Fake game window position: `-pos x,y`.
- Focus: the focus watcher runs for the whole run, so a test stalls in `focus` (`waiting_focus`) if the fake game loses focus. Dry-run skips `action.Guard`.
- Screens: `-screen N` (or `"screen": N` in the config) opens the app windows and the selection overlay on monitor N (1 = primary, others left to right); `-list-screens` prints the monitors as JSON. Test runs always use screen 2, to keep test windows separate from the primary desktop. Selection coordinates are absolute and can be negative; capture uses the virtual desktop bounds. `fakegame` takes the same `-screen` and a `-pos` relative to it.
- Recording: people use the Record button (`ui/presenter/record_presenter.go`), which starts the same `recorder.Session`. Anything a person does goes through the UI; CLI flags are for scripts and agents. `pixel-bot-go.exe -record <dir> [-window "<title>"]` saves frames of the saved selection, plus `input.jsonl` for that window, until Ctrl+C. It opens no app window and sends no input. The dashboard is at `127.0.0.1:6070`; `-view <path>` reopens a recording. Recordings go under the git-ignored `datasets/` or `fishing-recordings/`; never commit them.
- End-to-end procedure (build, launch the simulator and desktop app, drive the UI, read signals, judge a healthy cycle): `.claude/skills/run-app/SKILL.md`.

## Release and repository hygiene

- Every push to `main` triggers auto-release: next patch tag `vX.Y.Z` (or run the workflow manually with a `version` input such as `v1.1.0` to pick the tag; tags so far are v1.0.0 to v1.0.7), Windows amd64 build, `CHECKSUMS.txt`, GitHub Release. Put `skip-release` in the commit message for docs-only or non-shippable commits.
- History was purged of a large demo GIF and a test image and force-pushed (main and tags v1.0.0 to v1.0.7). Do not re-add large binaries (GIFs, videos, screenshots) to git. `.gitattributes` still routes `*.gif` through LFS; decide on LFS before adding GIFs, and prefer a release asset (for example an MP4) linked from the README.
- Old objects may remain under `refs/pull/1` until PR #1 is deleted or GitHub support clears them.
