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
| ESP32 port | Unset | COM port for the paired BLE keyboard. Requires the Interact key reel method and keyboard bindings for Cast and Interact. |
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
| Dark theme | on | Dark or light colours. |
| Share recordings | off | Shows the Recorder tab, to record yourself fishing and send it in. See [Record and share a session](../how-to/share-recordings.md). |

With **Share recordings** on, Settings also shows:

| Setting | Default | What it does |
| --- | --- | --- |
| Recording folder | datasets | Where new recordings are saved, next to Go Fish. |
| Record keys | on | Record your keys as well as your mouse. |

Settings are saved in `pixle_bot_config.json` next to the exe. Delete that file to go back to the defaults.
