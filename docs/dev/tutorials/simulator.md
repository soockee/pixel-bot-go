---
title: "First run with the simulator"
summary: "Run a full cast, bite and reel cycle against the fake game, without the game and without sending input."
order: 2
---

1. Launch the simulator with `go run ./cmd/fakegame` and the desktop app with `./run.ps1 -dry-run`, which builds the frontend and the app first. Keep the simulator window visible.
2. On the **Fishing** tab, pick **Fake Game** as the game window.
3. Start capture; watch state: `waiting_focus → searching`.
4. When `monitoring` appears, a match was found; observe ROI stability.
5. Wait for a splash and check the log for the recorded reel and recast actions.
6. Adjust `Threshold` only if false triggers occur or misses persist.

Compare the timestamps for one full response cycle with the simulator event log.
