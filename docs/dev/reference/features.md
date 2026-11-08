---
title: "Features"
summary: "Everything Go Fish does, in one list."
order: 1
---

* Watch a screen region for the bobber template.
* Multi-scale template matching with stride + refine pass.
* Response state machine (cast → search → monitor → reel → cooldown).
* Bite detection via grayscale ROI motion heuristics (`motion`), or the splash brightening the centre of the marker box (`splash`).
* Desktop window focused on fishing: the live capture with the locked bobber, the cycle, catches this run, and progress toward the session and failed-cast limits. Settings holds keys, run limits and the theme. With `"debug": true` a Recorder tab, the bite watch and the detection settings appear. The app finds the game window itself: it searches the window's client area and, once the bobber is locked, captures only its box.
* JSON config persistence and structured event logging.
* In-app update: a release build checks the latest GitHub Release at start, every hour and on **Check for updates** (Settings > Updates), and offers **Update to vX.Y.Z** in the top bar and in Settings. The update downloads only `pixel-bot-go-windows-amd64.exe` and `CHECKSUMS.txt` over HTTPS from that release's `github.com/soockee/pixel-bot-go/releases/download/<tag>/` URLs, refuses the build unless its SHA-256 matches, renames the running exe to `<exe>.old`, puts the new one in place and restarts with the same arguments. It is refused during a run, a recording or an upload. The version comes from `-ldflags -X github.com/soocke/pixel-bot-go/internal/update.Version=vX.Y.Z`, set by `auto-release.yml`; any other value (`dev` by default) skips the check.
* Google sign-in with role-based access: a real run needs a role with `fishing.run`, uploading a recording `recordings.upload`. The owner assigns roles; an account nobody gave a role may do nothing. Dry runs need no account, and without a service URL and client ID (config or build) sign-in is off and nothing is gated.
