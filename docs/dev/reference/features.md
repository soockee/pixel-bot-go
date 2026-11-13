---
title: "Features"
summary: "Everything Go Fish does, in one list."
order: 1
---

* Watch a screen region for the bobber template.
* Multi-scale template matching with stride + refine pass.
* Response state machine (cast → search → monitor → reel → cooldown).
* Bite detection (`splash_sound`): a splash at the centre of the marker box confirmed by the game's bite sound, or the splash alone when the game plays no bite sound.
* Desktop window focused on fishing: the live capture with the locked bobber, the cycle, reel inputs sent this run, and progress toward the session and failed-cast limits. Settings holds keys, run limits and the theme. With `"debug": true` a Recorder tab, the bite watch and the detection settings appear. The app finds the game window itself: it searches the window's client area and, once the bobber is locked, captures only its box.
* JSON config persistence and structured event logging.
* In-app update: a release build checks the latest GitHub Release at start, every hour and on **Check for updates** (Settings > Updates), and offers **Update to vX.Y.Z** in the top bar and in Settings. The update downloads only `pixel-bot-go-windows-amd64.exe` and `CHECKSUMS.txt` over HTTPS from that release's `github.com/soockee/pixel-bot-go/releases/download/<tag>/` URLs, refuses the build unless its SHA-256 matches, renames the stable launcher executable to `<exe>.old`, puts the new one in place and restarts with the same arguments. The launcher starts the app from a temporary executable named after a random fish species and exits; the main window uses that same species name. A later launch removes copies older than 24 hours. If temporary launch fails, the app falls back to its normal executable name. Updates are refused during a run or a recording. The version comes from `-ldflags -X github.com/soocke/pixel-bot-go/internal/update.Version=vX.Y.Z`, set by `auto-release.yml`; any other value (`dev` by default) skips the check.
