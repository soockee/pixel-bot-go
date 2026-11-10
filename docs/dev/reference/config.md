---
title: "Configuration"
summary: "Every setting in pixle_bot_config.json, its default and its limits."
order: 2
---

| Setting                     | Meaning                                     | Tradeoff                               |
| --------------------------- | ------------------------------------------- | -------------------------------------- |
| Threshold                   | Minimum NCC score considered a hit          | ↑ fewer false positives, ↓ sensitivity |
| Stride                      | Pixel step while scanning, capped at a quarter of the template short side | Above the cap it has no effect |
| Refine                      | Precise second pass around best coarse spot | Slight cost, better accuracy           |
| MinScale/MaxScale/ScaleStep | Scale search range                          | Wide + tiny step = heavier workload    |
| CastKey (`cast_key`)        | Unmodified key that casts: F1 to F12, a letter, a top-row digit, `Mouse4` or `Mouse5` | An unknown key halts the run           |
| ReelMethod (`reel_method`)  | How to interact with the bobber             | `right_click` by default, or `interact_key` with the marker locator |
| InteractKey (`interact_key`) | Native Interact binding for key-based reeling, using the same supported keys as CastKey | Empty by default. Key-based reeling requires a binding distinct from Cast and Stop. |
| ESP32Port (`esp32_port`) | Serial port for the ESP32 BLE keyboard and mouse, such as `COM3`, or `auto` | Empty uses Windows input. `auto` tries each USB serial port with the firmware handshake and uses the first that answers. Keys go through the board as HID taps, and `right_click` mode moves the pointer (relative reports, closed loop on `GetCursorPos`) and clicks through it, which needs firmware that answers `POINTER`. Mouse 4 and 5 bindings go through the board as HID mouse buttons 4 and 5 (`BUTTON`, probed with `BUTTONS`). |
| Background (`background`) | Keep fishing while another window is in front | Off by default. `Validate` turns it off without the `interact_key` reel or with an ESP32 port, and the Settings tab refuses mouse side buttons for Cast and Interact with it. Fixed per run in `bindGameWindow`. Keys go to the game window with `PostMessage` (`action.BeginPostedKeyHold`; F10 as `WM_SYSKEYDOWN`), frames come from `PrintWindow` with `PW_RENDERFULLCONTENT` (`WindowIdentity.Print`, which draws the whole client area per grab), and the focus watcher and `CanAct` only need the window open and not minimized. The guard still requires focus for the pointer, and idle moves are skipped. Needs the game's Sound in Background for the bite sound. |
| Reposition (`reposition`) | Movement script after 45–55 acknowledged reels | Off by default. Presses W, A, S or D through `activity`, and drags the camera (`cameraDrag`) when the input mode owns the pointer: ESP32 with `MOUSE`, or Windows input in `right_click` mode. Requires the Logout key, which a failed sequence uses. Replaces `reposition_key`. |
| IdleMoves (`idle_moves`) | Between-catch movement, one in 12 cooldowns | Off by default. A camera nudge (one in three, where `cameraDrag` allows) or a cursor wander along `wanderPath` from `CursorPos`. Replaces `idle_key`. |
| TargetPlayerKey (`target_player_key`) | Optional addon binding that selects a friendly soft target | Empty by default. Any key the Cast key accepts that no other action uses; update the addon. |
| MaxFailedCasts              | Casts in a row without a reel before halting | Default 5                             |
| MaxSessionMinutes           | Run length; halts at the next cast after it | Default 60                             |
| Schedule (`schedule`)      | Alternate scheduled play and logged-out breaks | Off by default |
| PlayMinMinutes / PlayMaxMinutes (`play_min_minutes`, `play_max_minutes`) | Inclusive play duration bounds | 20 and 45 minutes by default |
| BreakMinMinutes / BreakMaxMinutes (`break_min_minutes`, `break_max_minutes`) | Inclusive logged-out duration bounds | 5 and 15 minutes by default |
| LogoutKey (`logout_key`) | Key the addon binds to its `Logout` macro (`/logout`), sent in `Keys.lua` while `schedule` or `reposition` is on | Checked when a run starts with breaks or Reposition, not on save, so the switch can go on first. The screen is read from the addon's in-world blocks (`fishing.WorldSignalArea`, `fishing.InWorld`), which replace `character_png` and `in_world_png`. |
| Locator (`locator`)         | Technique that finds the bobber             | `marker` by default, or `color` (finds the bobber by its colours, without the addon, and is easily fooled); see `detect.LocatorNames` |
| BiteDetector (`bite_detector`) | Technique that recognises a bite        | `splash_sound`, the only one: white spray at the bobber followed within 0.8 s by the bite sound from the selected window's process; with the game silent for the whole run, or after three casts in a row whose splash brought no bite sound when no bite sound was heard, the splash alone. On the reviewed recording it catches 9 of 10 bites with no early one. A saved `motion`, `splash` or `histogram` (removed) loads as `splash_sound` |
| RecordDir (`record_dir`)    | Folder for new recordings                   | Default `datasets`                     |
| RecordKeys (`record_keys`)  | Record keys as well as the mouse            | Default true                           |
| OverlayX / OverlayY (`overlay_x`, `overlay_y`) | Where the overlay pill sits on the game window, each a share of the free space: 0 is the left or top edge, 1 the right or bottom edge | Default 0.5 and 0.01, the top centre; outside 0 to 1 resets to the default. Written by Settings → Overlay → Move overlay (`app/overlay.go`); not a settings form field. The pill is 380 by 44 px in both overlay modes (`app/overlay_place.go`, `frontend/src/overlay.css`) |
| CatchesTotal / FishingSecondsTotal (`catches_total`, `fishing_seconds_total`) | Catches and seconds of fishing on this computer, dry runs excluded | Added when a run ends (`app/support.go`): the fish Bobber Marker reported caught, or the reels with an addon older than 1.5.1; shown on the thank-you card. Not settings form fields |
| SupportMilestone / SupportMuted (`support_milestone`, `support_muted`) | The catch milestone the thank-you card last asked at, and whether the person said they support Go Fish already or not to ask again | The card asks at 100, 500, 1000, 2500, 5000 and every 5000 catches, only after a run with a catch and at most one "not hooked" per four catches (`ui/desktop/support.go`). Older `support_asked_at` and `support_opened_at` are ignored. Not settings form fields |
| Theme (`theme`)             | Window look, one of `config.Themes`: `graphite`, `tavern`, `arcane`, `frost`, `fel`, `tide` | Default `graphite`; unknown values fall back to it. Replaces `dark_mode`. Settings → Theme |

Set the Cast key in the setup wizard or under **Settings → Fishing keys**.
