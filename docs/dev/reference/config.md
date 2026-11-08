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
| ESP32Port (`esp32_port`) | Serial port for the ESP32 BLE keyboard, such as `COM3` | Empty uses Windows input. Requires `interact_key` mode and keyboard bindings for both actions. |
| StopKey (`stop_key`)        | Global hotkey that halts the run; accepts the same unmodified keys as CastKey | Hidden from other apps; read at start  |
| MaxFailedCasts              | Casts in a row without a reel before halting | Default 5                             |
| MaxSessionMinutes           | Run length; halts at the next cast after it | Default 60                             |
| Schedule (`schedule`)      | Alternate scheduled play and logged-out breaks | Off by default |
| PlayMinMinutes / PlayMaxMinutes (`play_min_minutes`, `play_max_minutes`) | Inclusive play duration bounds | 20 and 45 minutes by default |
| BreakMinMinutes / BreakMaxMinutes (`break_min_minutes`, `break_max_minutes`) | Inclusive logged-out duration bounds | 5 and 15 minutes by default |
| LogoutKey (`logout_key`) | Binding for a user-configured `/logout` macro | Required for scheduled breaks |
| CharacterPNG / InWorldPNG (`character_png`, `in_world_png`) | Exact PNG crops for character selection and the logged-in game | Both required for scheduled breaks |
| Locator (`locator`)         | Technique that finds the bobber             | `marker` by default, or `color` (finds the bobber by its colours, without the addon, and is easily fooled); see `detect.LocatorNames` |
| BiteDetector (`bite_detector`) | Technique that recognises a bite        | `motion` by default, `histogram` (colour histogram against the calm bobber), `splash`, or `splash_sound` (splash plus the bite sound; listens to the selected window's process) |
| RecordDir (`record_dir`)    | Folder for new recordings                   | Default `datasets`                     |
| RecordKeys (`record_keys`)  | Record keys as well as the mouse            | Default true                           |
| ShareRecordings (`share_recordings`) | Shows the Recorder tab and the Recording settings without debug mode | Default false; Settings → Data sharing |
| CloudURL (`cloud_url`)      | Upload service URL | Empty uses the one release builds carry; empty in both turns sign-in off |
| OAuthClientID / OAuthClientSecret (`oauth_client_id`, `oauth_client_secret`) | Google Desktop OAuth client the app signs in with | Not confidential for desktop apps; empty client ID uses the release build's client; empty in both turns sign-in off |

Set the cast key in **Settings → Keys**. It must differ from the Stop hotkey.
