---
title: "First run with the simulator"
summary: "Run a full cast, bite and reel cycle against the fake game, without the game and without sending input."
order: 2
---

1. Launch the simulator with `go run ./cmd/fakegame` and the desktop app with `./run.ps1 -dry-run`, which builds the frontend and the app first. Keep the simulator window visible.
2. On the **Fishing** tab, pick **Fake Game** as the game window.
3. Start capture; watch state: `waiting_focus → searching`.
4. When `monitoring` appears, a match was found; observe ROI stability.
5. Wait for a splash and check the log for the recorded reel and recast actions. The simulator plays a recorded bite sound shortly after each splash, so the log says `bite detected` with `sound_after_splash_ms`. A scenario file with `{"sound": "no_bite"}` plays no bite sound, as a game with sound effects off: the first cast runs out, then bites log `without_bite_sound`.
6. Adjust `Threshold` only if false triggers occur or misses persist.

Compare the timestamps for one full response cycle with the simulator event log.
