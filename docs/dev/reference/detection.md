---
title: "Detection"
summary: "What counts as the bobber and how the addon's brackets are found."
order: 6
---

## What Counts as the Bobber

The bobber is whatever an addon marks with four corner brackets, so other players' bobbers are ignored. The Bobber Marker addon in `addon/BobberMarker` draws them for the bot; the salmon brackets of NaowhForever's Glimmer, which older recordings show, are accepted too. `detect.MarkerLocator` looks for these in the capture:

| Property | Value |
| -------- | ----- |
| Colour | Magenta (255, 0, 255): red and blue at least 200, green at most 60. Or Glimmer's salmon, which blends with the scene: red at least 230, green at least 50 below it, green and blue within 35 |
| Bracket | Two arms of at least 10 px from one outer corner; Bobber Marker draws 20 px arms, 3 px thick |
| Box | 30 to 400 px per side, with at least 3 of 4 corners visible; Bobber Marker draws 96 px at full zoom-out, scaled with the camera distance |

### Bobber Marker addon

For WoW: Forever (Interface 16001). It shows the brackets only while you channel Fishing and the game's soft target is your bobber (game object 35591, in any language), so they vanish the moment you reel.

| Item | Value |
| ---- | ----- |
| Look | Opaque magenta brackets, sized in screen pixels whatever the UI scale |
| Position | Base centre 40 px below the bobber's nameplate. Box and offset hold at 15 yd of camera distance (`GetCameraZoom`) and scale by (15 + 20) / (zoom + 20), clamped to 0.5 to 4, because the bobber looks bigger and further below its nameplate up close; a change over 5 % reattaches the marker. Each cast adds an offset of up to 2 px per axis and keeps it through any reattach. `/bobbermarker y <pixels>` changes the base vertical offset (negative is down) in `BobberMarkerDB` |
| Set each time you start fishing (out of combat), when they differ | `SoftTargetInteract` 3, `SoftTargetIconGameObject` 1, `SoftTargetInteractOnlyInRange` 0, `SoftTargetInteractRange` 60 |
| Set while you fish | `SoftTargetNameplateInteract` 1, then back to your value |
| Keys | `Keys.lua` is written by `addon.Install` from the config, not carried: `BobberMarkerKeys = { cast, interact }` in the game's binding names (`addon.BindingKey`). The Interact key is set only for the `interact_key` reel method. `addon.State` compares it, so a changed key makes the addon outdated |
| Set at login, and when combat ends | An account macro `Fishing` (`#showtooltip` and `/cast` with the name of spell 7620, so it follows the game's language; an existing macro of that name is rewritten) bound with `SetBindingMacro` to the cast key, and `INTERACTTARGET` bound with `SetBinding` to the interact key. Only what differs is changed, never in combat, and nothing is written to the chat; problems are kept and `/bobbermarker` prints them |

Console variables persist in `WTF/Config.wtf`, so disabling the addon does not undo the set-once values. Glimmer turns the same nameplate on and off for its own highlights, so switch off its Fishing Bobber highlight while you use Bobber Marker.

The box centre is the click point and the box is the area watched for a bite. The whole box must fit inside the game window. If the addon is off or its colour changes, nothing is found and the run halts after `max_failed_casts`.
