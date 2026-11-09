---
title: "Settings"
summary: "Every setting on the Settings tab, with its default."
order: 1
---

| Setting | Default | What it does |
| --- | --- | --- |
| **Fishing** | | |
| Cast key | F3 | The key or mouse side button Go Fish presses to cast Fishing. Must match the game. |
| Reel method | Right click | Right-click the marked bobber, or press your Interact key without moving the cursor. |
| Interact key | Unset | Set this to match **Interact With Target** in WoW before choosing the Interact reel method. It must differ from the Cast key. |
| **Input** | | |
| ESP32 board | Off | **Auto**, or the board's COM port such as `COM3`, to press keys and click through an ESP32 board. See [Change keys and limits](../how-to/settings.md#esp32-board). |
| **Run** | | |
| Session length | 60 min | Go Fish stops at the first cast after this time. 5 to 240 minutes. |
| Casts without a bite | 5 | Go Fish stops after this many casts in a row without a catch. 1 to 20. |
| Pause after a catch | Quick | Wait before the next cast. **Quick** casts again after a random 0.4 to 1.2 seconds, like a player; or pick 1 to 60 seconds. The next search ignores the old bobber. |
| Cast length | 30 s | Cast again when nothing bites within this time. 5 to 180 seconds. |
| **Between catches** | | |
| Move the mouse | Off | Now and then after a catch, the cursor drifts a little along a curved path, or the camera turns slightly. |
| Change spot | Off | Every 45 to 55 catches, Go Fish takes a step with W, A, S or D, or turns the camera, and checks the next cast. Needs the Logout key, to log out if it gets stuck. |
| Target passing player | Off | A spare key Bobber Marker binds to select a friendly player who walks past while you fish. |
| **Scheduled breaks** | | |
| Take breaks | Off | Alternates random fishing sessions with logged-out breaks after Start. |
| Shortest / longest session | 20 / 45 min | Shown with breaks on: the range for each fishing session. |
| Shortest / longest break | 5 / 15 min | Shown with breaks on: the range for each logged-out break. |
| Logout key | Unset | Shown with breaks or Change spot on: a spare key. The addon puts a **Logout** macro on it after an **Update addon** and `/reload`. |
| Move overlay | Top centre | A button, not a setting you type: drag the pill that shows a run over the game, then click **Done** to keep its place. See [Change keys and limits](../how-to/settings.md#move-the-overlay). |
| Theme | Graphite | The window's look, words, icon and catch sound: Graphite, Tavern Ledger, Arcane Grimoire, Frostwater, Felfire Forge or Tidecaller. |
| Theme sounds | on | Plays the theme's sound on each catch. Kept on this computer only. |

With **Change spot** on, Go Fish moves after 45 to 55 catches. When Go Fish
owns the cursor (the Right click reel method, or any method with an ESP32), it
may turn the camera with a short mouse drag instead of a step. If no bobber
appears after the first move, it tries three to five more times. If it still
cannot fish, it stops and logs out. Scheduled play does not resume after that.

To use **Target passing player**, click it and press a spare key, update
Bobber Marker, and type `/reload` in the game. The addon turns on friendly soft
targeting and watches for a friendly player in range while you fish. Click
**Clear**, update Bobber Marker and reload to turn it off again.

To go back to all the defaults, close Go Fish and delete `pixle_bot_config.json` next to it.
