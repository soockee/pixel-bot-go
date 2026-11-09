# AGENTS.md

Go Fish is a Windows Go application for visual event and response experiments. It captures a screen region, locates the target from the corner brackets an addon draws around it, and uses region-of-interest brightness statistics to trigger action callbacks. The local simulator supplies a scripted fishing scene for reproducible tests. See [README.md](README.md) and `docs/` (Diataxis: tutorials, how-to, reference, explanation; published at https://soockee.github.io/pixel-bot-go/).

## Stack

- Go 1.26. Module path is `github.com/soocke/pixel-bot-go` (one `e`), which differs from the GitHub owner `soockee`; use the `go.mod` spelling in imports.
- UI: Wails v3 (`github.com/wailsapp/wails/v3`, WebView2, no cgo) with a Vite + TypeScript frontend in `frontend/`. Win32 access via `golang.org/x/sys`.
- Capture and input injection (`domain/capture/capture_windows.go`, `domain/action/actions_windows.go`) are `_windows.go` files. Builds and runs on Windows only. It needs Administrator only when the game runs elevated (UIPI blocks input into a higher-integrity process) or the game folder is not writable.

## Commands

```bash
./run.ps1 [app flags]               # PowerShell: build frontend and app, then start it (e.g. ./run.ps1 -dry-run)
(cd frontend && npm ci && npm run build)   # frontend, embedded by go build; needed once and after frontend changes
(cd frontend && npm run bindings)   # regenerate frontend/bindings after changing ui/desktop types or Service methods (needs wails3 CLI v3.1.0)
go build -o pixel-bot-go.exe .      # build (console program; releases and run.ps1 add -ldflags '-H=windowsgui', so no terminal opens)
./scripts/theme-icons.ps1            # after changing frontend/public/themes/*.svg: renders app/icons/*.png and the exe icon (rsrc_windows_amd64.syso, needs Edge and wails3)
go test ./...                       # run all tests
go vet ./...                        # static checks
gofmt -l .                          # must print nothing
GOOS=windows GOARCH=amd64 go build ./...   # cross-compile check from non-Windows
```

There is no Makefile. Without a frontend build the window is empty (`frontend/dist` holds only `.gitkeep`). `.github/workflows/ci.yml` builds the frontend, then runs gofmt, vet, tests and the race detector on Windows for pull requests, and `auto-release.yml` runs it before every release. The race step stays non-blocking until it has a clean run.

## Layout

| Path | Role |
|------|------|
| `main.go`, `logger.go` | Entry point; loads `pixle_bot_config.json` (sic, the filename is misspelled on purpose, keep it), sets up slog, optional pprof on `localhost:6060` when `debug` is true |
| `app/` | Wails application and window (`app.go`), and `AppContainer` (`container.go`) that wires models, presenters and the desktop adapter. `overlay.go` is the transparent, click-through status window over the game, hidden from screen capture and parked off screen unless a run goes; Settings → Overlay → Move overlay shrinks it to a draggable pill (`overlay_place.go` holds the placement maths) and saves its place as `overlay_x`, `overlay_y` |
| `config/` | `Config` struct, `DefaultConfig`, `Validate`, `Load` (JSON) |
| `domain/capture/` | `Source.Next` grabs one frame of the selected region; `Frame.Origin` is its screen position |
| `domain/detect/` | `Locator` and `BiteDetector` interfaces, `MarkerLocator` (the app's locator), NCC template matching (evaluation only), `SplashSoundDetector` (the only bite detector: splash plus bite sound, splash alone when the game plays none) |
| `domain/fishing/` | `Engine` (`engine.go`, the capture and detection loop) and the FSM (`fsm.go`: states, deadline, input) |
| `domain/action/` | Key press, cursor move, right click (Windows input injection), `Guard` (safety checks on real input) |
| `ui/model` `ui/presenter` | MVP-style UI: models hold state, presenters connect domain to view interfaces. No toolkit dependency |
| `ui/desktop` | Adapter between presenters and the window, no toolkit dependency: `View` turns presenter calls into one `State`, `Service` is what the frontend calls, `Loop` runs both on one goroutine, plus settings validation, selection, recordings and preview frames, and the setup steps (`setup.go`: what is still missing before a run, computed from `State` in `View.Flush`; `State` is compared by value, so it holds arrays, not slices) |
| `frontend/` | Vite + TypeScript frontend (`src/`: `live.ts` the Fishing view, `setup.ts` its "Set up fishing" wizard, shown in place of the preview until setup is done, `recorder.ts`, `settings.ts`). The Fishing view is the product; the bite watch and detection settings are developer tools shown only when `State.Debug` (config `debug`) is set; the Recorder tab and Recording settings are developer tools too. `bindings/` is generated from `ui/desktop`; `assets.go` embeds `dist/` |
| `internal/update/` | In-app update: latest GitHub Release, checksum-verified download, exe swap and restart; `Version` is set by ldflags in `auto-release.yml` (`app/update.go` wires it) |
| `debug/` | `/debug/state` handler |
| `addon/` | `BobberMarker/`: the WoW: Forever addon (Lua) that draws the magenta brackets `MarkerLocator` finds, only while fishing. No Lua tooling in the repo; check it in the client. `addon.go` embeds it, so the app installs and updates it in the game (`app/addon.go`, Settings → Addon); the game folder comes from the game window's exe (`capture.WindowExe`) or the default install path. Install also writes `Keys.lua` with the Cast and Interact keys from the config, which the addon binds in the game (a macro and `INTERACTTARGET`); changing either key makes the addon outdated |
| `assets/` | Embedded `fishing_target.png` template and the game's recorded bite sounds `sounds/*.ogg` (`embed.go`; `BiteSoundPCM` decodes them to the capture layout), used by the simulator, NCC evaluation and the bite detector tests |
| `cmd/detecteval/` | Compares every registered `Locator` and `BiteDetector` on recorded datasets, writes `labels.json` from a session with `-label`, and exports each cast's watched area with `-export`. See `docs/dev/reference/datasets.md` |
| `internal/dataset/` | Dataset format, labels and the frame loop |
| `internal/recorder/` | `-record` sessions (frames plus mouse and key input in one window) and `ReadEvents` for the Recorder tab |
| `cmd/responsebench/` | Standalone virtual-clock benchmark of pacing and retry policies; run `go run ./cmd/responsebench`. See `docs/dev/reference/benchmark.md`. Output goes to the git-ignored `responsebench-results/` |

## Architecture notes

- FSM lifecycle: `waiting_focus → searching → monitoring → reeling → cooldown → casting → searching`. A single event channel serializes transitions; timers decouple cadence from UI ticks. Add new event sources by feeding that channel rather than mutating state directly. Each state has at most one deadline (`arm`), a transition cancels it, and input actions run on the loop, so nothing pending can act after Stop or a focus change.
- `fishing.Engine` runs the pipeline on one goroutine with no UI dependency: it grabs a frame, runs the detection the FSM state calls for, and sends the result to the FSM. The UI only reads `Engine.Preview` on its tick.
- Techniques are exchangeable: implement `detect.Locator` or `detect.BiteDetector` and add one line to `domain/detect/registry.go`. The `locator` and `bite_detector` config values pick them in the app and config panel, and `detecteval` compares them all by default.
- Algorithms plug in through `detect.Locator` and `detect.BiteDetector` (a `detect.SoundBiteDetector` also gets the game's sound through `fishing.SoundSource`), frames through `fishing.FrameSource`, and input through `fishing.ActionCallbacks`, so tests substitute fakes instead of touching Win32. Keep those seams when adding input, capture or detection behavior. Define other interfaces where they are consumed.
- The bobber is whatever the addon marks: `detect.MarkerLocator` returns the box inside its corner brackets, the box centre is the click point, and the box is the area watched for a bite. Without the addon the bot finds nothing and halts after `max_failed_casts`.
- UI and logic: the frontend only renders `desktop.State` (the `state` event) and calls `desktop.Service`; it holds no rules. Validation, defaults and decisions live in Go (`ui/desktop`, presenters, domain). `ui/desktop` and `ui/presenter` do not import Wails; only `app/` and `frontend/` know the toolkit. Add a field to `State` or a method to `Service`, regenerate the bindings, then render it.
- Build all wiring in `app/container.go`; do not construct services inside presenters or views. Constructors require their dependencies, so do not add nil guards for them.

## Conventions

- Follow `.github/instructions/go.instructions.md` (idiomatic Go, early returns, documented exports, no emoji, English comments, exactly one `package` line per file when editing).
- Tests are table-friendly `_test.go` files beside the code (`fsm_test.go`, `engine_test.go`, `bite_detector_test.go`, presenter/model tests). Add or update tests for FSM, detector and presenter changes; run `go test ./...` before finishing.
- Windows-specific code goes in `*_windows.go` files.
- Themes: Go keeps the ids (`config.Themes`) and the choice; `frontend/src/themes.ts` holds each theme's words, catch sound and burst, `frontend/src/themes.css` its look (it reacts to the fishing step through `body[data-fsm]`), `frontend/public/themes/<id>.svg` its icon. A new theme touches all four plus `scripts/theme-icons.ps1` output.
- Config changes: update `Config`, `DefaultConfig`, `Validate`, and `docs/dev/reference/config.md` together, plus `docs/reference/settings.md` when the setting is on the Settings tab.
- Docs follow Diataxis. `docs/tutorials`, `docs/how-to`, `docs/reference` and `docs/explanation` are the player guide, plain language with nothing technical, published by GitHub Pages (Jekyll: `_config.yml`, `_layouts/`, `assets/`) at https://soockee.github.io/pixel-bot-go/. Each page has `title`, `summary` (shown as "In short") and `order` front matter. `docs/dev/` holds the technical docs in the same four folders and is not published. Put new content on the matching page or add one; keep the player guide's data sharing page true when what the app sends changes.

## Agentic testing and signals

- Logs: the app writes JSON slog entries to `pixel_bot_logs.json` (truncated each start). FSM transitions are logged at Info (`fishing state transition`, fields `from`/`to`), plus `cast action executed` and `reel action executed`.
- Live state: with `"debug": true` in `pixle_bot_config.json`, `http://localhost:6060/debug/state` returns JSON (`fsm_state`, `capture_running`, `selected_window`, `target`, `uptime_seconds`, `time`) next to the pprof endpoints. Implemented in `debug/state.go`, registered in `app.Run`.
- Driving the UI: in debug mode the WebView2 DevTools listen on `localhost:9222` (`curl localhost:9222/json` lists the page). Use the DevTools protocol (`Runtime.evaluate` to click elements such as `#tab-recorder` or `#start`, `Page.captureScreenshot` for a picture).
- Replay tests: `domain/fishing/engine_test.go` runs rendered fake-game frames through the Engine, real matcher, real bite detector and FSM with recorded fake actions, without a window. Extend it for new detection or FSM behavior. FSM and Engine tests use `testing/synctest`, so timers run on a fake clock.
- Fake game: `go run ./cmd/fakegame [-scenario file.json] [-title "Fake Game"]` opens a frameless window with a noisy scene, the real bobber template with the addon's brackets and scripted bites (default loop: empty 2s, bobber 4s, splash 0.6s, empty 3s). It plays the game's sound from its own process, so the app's process loopback hears it as it hears the game (sound in a WebView would come from a child process the loopback misses): a quiet hum and one of the recorded bite sounds in `assets/sounds/` 0.2 to 0.45 s after each splash. Each phase change and each bite sound prints a JSON line on stdout (`t`, `phase`, `bobber` rect, or `sound`), to correlate with `pixel_bot_logs.json`. Scene logic lives in `internal/fakegame` and is unit-tested against the real template matcher and bite detector. Scenario JSON fields: `width`, `height`, `bobber_x`, `bobber_y`, `bobber_scale`, `seed`, `loop`, `splash_lum`, `bobber_spread` (each new bobber lands up to that many pixels from home), `sound` (`bite` by default, `no_bite` for the hum only as with sound effects off, `off` for silence), `timeline` (`[{"phase":"empty|bobber|splash","ms":N}]`). `-follow <app -trace-out file>` draws the dry run's pointer trail in a capture-excluded window; `responsebench -cursor-audit` compares those paths with recordings (`docs/dev/reference/input-models.md`).
- Dry run: `pixel-bot-go.exe -dry-run` (or `PIXELBOT_DRY_RUN=1`) logs key presses, cursor moves and clicks as `dry-run action` entries instead of injecting them; no Administrator needed. `Config.DryRun` is `json:"-"` so it is never saved. Implemented in `domain/action/dryrun.go`, wired in `app/container.go`.
- Capture area: there is no selection. The app captures the game window's client area (`capture.WindowArea`, physical pixels) while it searches, and only the locked bobber's box while it watches (`fishing.Engine.step`); every capture costs about 10 ms whatever its size, so watching stays at 30 fps. The preview keeps the last window capture and shows the live box on it.
- Headless run: `-auto-window "<title>" -auto-start` preselects the game window and starts capture 0.5 s after launch. Search results are logged at Debug as `detection.search` (`found`, `score`, `scale`, `x`, `y`). Fake game window position: `-pos x,y`.
- Focus: the focus watcher runs for the whole run, so a test stalls in `focus` (`waiting_focus`) if the fake game loses focus. Dry-run skips `action.Guard`.
- Screens: `-screen N` (or `"screen": N` in the config) opens the app window on monitor N (1 = primary, others left to right); `-list-screens` prints the monitors as JSON. Test runs always use screen 2, to keep test windows separate from the primary desktop. Screen coordinates are absolute and can be negative; capture uses the virtual desktop bounds. A run is refused when the game window is outside the configured screen. `fakegame` takes the same `-screen` and a `-pos` relative to it.
- Recording: people use the Record button (`ui/presenter/record_presenter.go`), which starts the same `recorder.Session`. Anything a person does goes through the UI; CLI flags are for scripts and agents. `pixel-bot-go.exe -record <dir> -window "<title>"` saves frames of that window's client area, fixed where it is at the start, plus `input.jsonl` and the process's sound in `audio.wav` (`capture.CaptureAudio`, process loopback) for that window, until Ctrl+C. It opens no app window and sends no input. The Recorder tab shows recordings, live while they record. Recordings go under the git-ignored `datasets/` or `fishing-recordings/`; never commit them.
- End-to-end procedure (build, launch the simulator and desktop app, drive the UI, read signals, judge a healthy cycle): `.claude/skills/run-app/SKILL.md`.

## Release and repository hygiene

- Every push to `main` triggers auto-release: next patch tag `vX.Y.Z` (or run the workflow manually with a `version` input such as `v1.1.0` to pick the tag; tags so far are v1.0.0 to v1.0.7), Windows amd64 build, `CHECKSUMS.txt`, GitHub Release. Put `skip-release` in the commit message for docs-only or non-shippable commits.
- History was purged of a large demo GIF and a test image and force-pushed (main and tags v1.0.0 to v1.0.7). Do not re-add large binaries (GIFs, videos, screenshots) to git. `.gitattributes` still routes `*.gif` through LFS; decide on LFS before adding GIFs, and prefer a release asset (for example an MP4) linked from the README.
- Old objects may remain under `refs/pull/1` until PR #1 is deleted or GitHub support clears them.
