---
title: "Settings"
summary: "Every setting on the Settings tab, with its default."
order: 1
---

| Setting | Default | What it does |
| --- | --- | --- |
| Cast key | F3 | The key or mouse side button Go Fish presses to cast Fishing. Must match the game. |
| Reel method | Right click | Right-click the marked bobber, or use the game's native Interact binding without moving the cursor. |
| Interact key | Unset | The binding used when the reel method is Interact key. It must differ from the Cast and Stop keys. |
| ESP32 port | Unset | `auto` to find the paired BLE keyboard on its USB cable, or its COM port such as `COM3`. Requires the Interact key reel method and keyboard bindings for Cast and Interact. |
| Reposition key | Unset | Enter `auto` for short WASD movements or enter a game key bound to a small step or turn. Go Fish checks the next cast after moving. Set the Logout key and both screen images first. |
| Between-catch action | Unset | Occasionally presses a configured one-shot game action after a catch. Use an unused key and bind the desired action in the game. |
| Target passing player | Unset | Selects a nearby friendly player offered by the game's soft targeting and continues fishing. |
| Stop key | F10 | Stops Go Fish from anywhere. Takes effect after a restart. |
| Session length | 60 min | Go Fish stops at the first cast after this time. 5 to 240 minutes. |
| Schedule play and breaks | Off | Alternates random fishing sessions and logged-out breaks after Start. |
| Shortest / longest session | 20 / 45 min | Inclusive range for each scheduled fishing session. |
| Shortest / longest break | 5 / 15 min | Inclusive range for each logged-out break. |
| Logout key | Unset | Key bound to the in-game `/logout` macro. |
| Character screen image / In-game image | Unset | Paths to PNG crops used to confirm the game screens. |
| Casts without a bite | 5 | Go Fish stops after this many casts in a row without a catch. 1 to 20. |
| Pause after a catch | 8 s | Wait before the next cast. 1 to 60 seconds. |
| Cast length | 30 s | Cast again when nothing bites within this time. 5 to 180 seconds. |
| Move overlay | Top centre | A button, not a setting you type: drag the pill that shows a run over the game, then click **Done** to keep its place. See [Change keys and limits](../how-to/settings.md#move-the-overlay). |
| Dark theme | on | Dark or light colours. |
| Share recordings | off | Shows the Recorder tab, to record yourself fishing and send it in. See [Record and share a session](../how-to/share-recordings.md). |

With **Reposition key** enabled, Go Fish moves after 45–55 acknowledged reels. In
Windows right-click mode or with updated ESP32 firmware, automatic repositioning
can also drag the camera horizontally when the pointer is inside the game.
Older ESP32 firmware uses WASD. If no bobber appears after the first move, it tries three to
five more times. If it still cannot fish, it stops and tries to log out
using the saved screen images. Scheduled play does not resume after that
logout.

To use **Target passing player**, enter an unused key, update Bobber Marker,
and type `/reload` in the game. The addon enables friendly soft targeting and
checks for a friendly player in range while you are fishing. Clear the key,
update Bobber Marker, and reload to disable this action.

With **Share recordings** on, Settings also shows:

| Setting | Default | What it does |
| --- | --- | --- |
| Recording folder | datasets | Where new recordings are saved, next to Go Fish. |
| Record keys | off | Keep a local key trace alongside mouse activity. Reviewed uploads exclude key events. |

Settings are saved in `pixle_bot_config.json` next to the exe. Delete that file to go back to the defaults.
