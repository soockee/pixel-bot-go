---
title: "Settings"
summary: "Every setting on the Settings tab, with its default."
order: 1
---

| Setting | Default | What it does |
| --- | --- | --- |
| **Fishing keys** | | |
| Cast key | F9 | The key or mouse side button Go Fish presses to cast Fishing. Must match the game. |
| Water walking | Off | Keeps Water Walking, Levitate or the Elixir of Water Walking up, so you can fish on water: the Cast key renews the buff once when it is gone or about to end. See [Change keys and limits](../how-to/settings.md#fishing-keys). |
| **Reeling** | | |
| Reel method | Right click | Right-click the marked bobber, or press your Interact key without moving the cursor. |
| Interact key | F10 | A spare key for the Interact reel method. Bobber Marker binds it to **Interact With Target** in WoW. It must differ from the Cast key. |
| **Input** | | |
| ESP32 board | Choose a board | Required for live fishing. Choose **Auto** to find the board or select its COM port, such as `COM3`. **Test** checks it over USB and Bluetooth. See [ESP32 setup](../how-to/settings.md#esp32-board). |
| **Run** | | |
| Session length | 60 min | Go Fish stops at the first cast after this time. 5 to 240 minutes. |
| Casts without a bite | 5 | Go Fish stops after this many casts in a row without a catch. 1 to 20. |
| Pause after a catch | Quick | Wait before the next cast. **Quick** casts again after a random 0.4 to 1.2 seconds, like a player; or pick 1 to 60 seconds. The next search ignores the old bobber. |
| Cast length | 30 s | Cast again when nothing bites within this time. 5 to 180 seconds. |
| **Between catches** | | |
| Move the mouse | Off | After every catch, before the next cast, the cursor drifts along a curved path, and about one time in four it circles the whole game window. Sometimes the camera turns slightly instead, and the next turn puts it back. It never clicks or holds a button, and it stops as soon as the next cast is due, so it never gets in the way of watching or reeling. |
| Change spot | Off | Every 45 to 55 catches, Go Fish takes a step with W, A, S or D, or turns the camera, and checks the next cast. Needs the Logout key, to log out if it gets stuck. |
| **Scheduled breaks** | | |
| Take breaks | Off | Alternates random fishing sessions with logged-out breaks after Start. |
| Shortest / longest session | 20 / 45 min | Shown with breaks on: the range for each fishing session. |
| Shortest / longest break | 5 / 15 min | Shown with breaks on: the range for each logged-out break. |
| Logout key | F12 | Shown with breaks or Change spot on: a spare key. The addon puts a **Logout** macro on it after an **Update addon** and `/reload`. |
| Move overlay | Top centre | A button, not a setting you type: drag the pill that shows a run over the game, then click **Done** to keep its place. See [Change keys and limits](../how-to/settings.md#move-the-overlay). |

With **Change spot** on, Go Fish moves after 45 to 55 catches. If the board
supports camera drags and the pointer is inside the game window, Go Fish may
turn the camera with a short mouse drag instead of a step. If no bobber
appears after the first move, it tries three to five more times. If it still
cannot fish, it stops and logs out. Scheduled play does not resume after that.

To go back to all the defaults, close Go Fish and delete `pixle_bot_config.json` next to it.
