# AGENTS.md

Pixel Bot: a Windows-only World of Warcraft fishing helper written in Go. It captures a screen region, finds the bobber via multi-scale normalized cross-correlation (NCC) template matching, detects a bite from ROI brightness statistics, then reels and recasts. See [README.md](README.md) and `docs/` (Diataxis: TUTORIALS, HOW_TO, REFERENCE, EXPLANATION).

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

There is no Makefile and no test/lint CI. The only workflow is `.github/workflows/auto-release.yml`.

## Layout

| Path | Role |
|------|------|
| `main.go`, `logger.go` | Entry point; loads `pixle_bot_config.json` (sic, the filename is misspelled on purpose, keep it), sets up slog, optional pprof on `localhost:6060` when `debug` is true |
| `app/` | `App` wrapper and `AppContainer` (`container.go`) that wires models, services, presenters and the root view |
| `config/` | `Config` struct, `DefaultConfig`, `Validate`, `Load` (JSON) |
| `domain/capture/` | Screen capture service, NCC (`ncc.go`), multi-scale matching, detection, metrics |
| `domain/fishing/` | Fishing FSM (`fsm.go`), bite detector, shared types/contracts |
| `domain/action/` | Key press, cursor move, right click (Windows input injection) |
| `ui/model` `ui/presenter` `ui/view` | MVP-style UI: models hold state, presenters connect domain to views, views are Tk widgets |
| `ui/images`, `ui/theme` | ROI extraction/scaling helpers, theming |
| `debug/` | Goroutine and memstats loggers |
| `assets/` | Embedded `fishing_target.png` template (`embed.go`) |

## Architecture notes

- FSM lifecycle: `waiting_focus → searching → monitoring → reeling → cooldown → casting → searching`. A single event channel serializes transitions; timers decouple cadence from UI ticks. Add new event sources by feeding that channel rather than mutating state directly.
- Dependencies are injected through interfaces (`fishing.FishingFSMContract`, `fishing.BiteDetectorContract`, `capture.CaptureService`) and `fishing.ActionCallbacks`, so tests substitute fakes instead of touching Win32. Keep that seam when adding input/capture behavior.
- Build all wiring in `app/container.go`; do not construct services inside presenters or views.

## Conventions

- Follow `.github/instructions/go.instructions.md` (idiomatic Go, early returns, documented exports, no emoji, English comments, exactly one `package` line per file when editing).
- Tests are table-friendly `_test.go` files beside the code (`fsm_test.go`, `bite_detector_test.go`, presenter/model tests). Add or update tests for FSM, detector and presenter changes; run `go test ./...` before finishing.
- Windows-specific code goes in `*_windows.go` files.
- Config changes: update `Config`, `DefaultConfig`, `Validate`, and `docs/REFERENCE.md` together.
- Docs follow Diataxis; put new content in the matching file.

## Agentic testing and signals

- Logs: the app writes JSON slog entries to `pixel_bot_logs.json` (truncated each start). FSM transitions are logged at Info (`fishing state transition`, fields `from`/`to`), plus `cast action executed` and `reel action executed`.
- Live state: with `"debug": true` in `pixle_bot_config.json`, `http://localhost:6060/debug/state` returns JSON (`fsm_state`, `capture_running`, `selected_window`, `target`, `uptime_seconds`, `time`) next to the pprof endpoints. Implemented in `debug/state.go`, registered in `app.Run`.
- Replay tests: `domain/fishing/replay_test.go` feeds synthetic frame sequences through the FSM and the real bite detector with recorded fake actions. Extend it for new detection or FSM behavior; helpers `synthFrame`/`applyRegion` live in `bite_detector_test.go`.
- Fake game: `go run ./cmd/fakegame [-scenario file.json] [-title "Fake Game"]` opens a window with a noisy scene, the real bobber template and scripted bites (default loop: empty 2s, bobber 4s, splash 0.6s, empty 3s). Each phase change prints a JSON line on stdout (`t`, `phase`, `bobber` rect), to correlate with `pixel_bot_logs.json`. Scene logic lives in `internal/fakegame` and is unit-tested against the real template matcher and bite detector. Scenario JSON fields: `width`, `height`, `bobber_x`, `bobber_y`, `bobber_scale`, `seed`, `loop`, `splash_lum`, `timeline` (`[{"phase":"empty|bobber|splash","ms":N}]`).
- Dry run: `pixel-bot-go.exe -dry-run` (or `PIXELBOT_DRY_RUN=1`) logs key presses, cursor moves and clicks as `dry-run action` entries instead of injecting them; no Administrator needed. `Config.DryRun` is `json:"-"` so it is never saved. Implemented in `domain/action/dryrun.go`, wired in `app/container.go`.
- Headless run: `-auto-window "<title>" -auto-start` preselects the target window and starts capture 0.5 s after launch; the capture region comes from `selection_x/y/w/h` in `pixle_bot_config.json`. Search results are logged at Debug as `detection.search` (`found`, `score`, `scale`, `x`, `y`). Fake game window position: `-pos x,y`.
- End-to-end procedure (build, launch fakegame plus bot, drive the UI, read signals, judge a healthy cycle): `.claude/skills/run-app/SKILL.md`.

## Release and repository hygiene

- Every push to `main` triggers auto-release: next patch tag `vX.Y.Z`, Windows amd64 build, `CHECKSUMS.txt`, GitHub Release. Put `skip-release` in the commit message for docs-only or non-shippable commits.
- History was purged of a large demo GIF and a test image and force-pushed (main and tags v1.0.0 to v1.0.7). Do not re-add large binaries (GIFs, videos, screenshots) to git. `.gitattributes` still routes `*.gif` through LFS; decide on LFS before adding GIFs, and prefer a release asset (for example an MP4) linked from the README.
- Old objects may remain under `refs/pull/1` until PR #1 is deleted or GitHub support clears them.
- Automation of the game is against most server rules; keep README disclaimers intact.
