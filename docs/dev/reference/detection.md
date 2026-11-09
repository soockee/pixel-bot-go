---
title: "Detection"
summary: "What counts as the bobber, how it is found, and the matching formulas."
order: 6
---

## What Counts as the Bobber

The bobber is whatever an addon marks with four corner brackets, so other players' bobbers are ignored. The Bobber Marker addon in `addon/BobberMarker` draws them for the bot; the salmon brackets of NaowhForever's Glimmer, which older recordings show, are accepted too. `detect.MarkerLocator` looks for these in the capture:

| Property | Value |
| -------- | ----- |
| Colour | Magenta (255, 0, 255): red and blue at least 200, green at most 60. Or Glimmer's salmon, which blends with the scene: red at least 230, green at least 50 below it, green and blue within 35 |
| Bracket | Two arms of at least 10 px from one outer corner; Bobber Marker draws 20 px arms, 3 px thick |
| Box | 30 to 400 px per side, with at least 3 of 4 corners visible; Bobber Marker draws 96 px |

### Bobber Marker addon

For WoW: Forever (Interface 16001). It shows the brackets only while you channel Fishing and the game's soft target is your bobber (game object 35591, in any language), so they vanish the moment you reel.

| Item | Value |
| ---- | ----- |
| Look | Opaque magenta brackets, sized in screen pixels whatever the UI scale |
| Position | Base centre 40 px below the bobber's nameplate. Each cast adds an offset of up to 2 px per axis and keeps it through any reattach. `/bobbermarker y <pixels>` changes the base vertical offset (negative is down) in `BobberMarkerDB` |
| Set once, the first time you fish | `SoftTargetInteract` 3, `SoftTargetIconGameObject` 1, `SoftTargetInteractOnlyInRange` 0, `SoftTargetInteractRange` 60 |
| Set while you fish | `SoftTargetNameplateInteract` 1, then back to your value |
| Keys | `Keys.lua` is written by `addon.Install` from the config, not carried: `BobberMarkerKeys = { cast, interact }` in the game's binding names (`addon.BindingKey`). The Interact key is set only for the `interact_key` reel method. `addon.State` compares it, so a changed key makes the addon outdated |
| Set at login, and when combat ends | An account macro `Fishing` (`#showtooltip` and `/cast` with the name of spell 7620, so it follows the game's language; an existing macro of that name is rewritten) bound with `SetBindingMacro` to the cast key, and `INTERACTTARGET` bound with `SetBinding` to the interact key. Only what differs is changed, never in combat, and nothing is written to the chat; problems are kept and `/bobbermarker` prints them |

Console variables persist in `WTF/Config.wtf`, so disabling the addon does not undo the set-once values. Glimmer turns the same nameplate on and off for its own highlights, so switch off its Fishing Bobber highlight while you use Bobber Marker.

The box centre is the click point and the box is the area watched for a bite. The whole box must fit inside the game window. If the addon is off or its colour changes, nothing is found and the run halts after `max_failed_casts`. `roi_size_px` and the matching settings apply only to `detecteval -locator ncc`.

## Algorithm Flow

```text
generate scale list
for each scale in parallel:
    use the template prepared for that scale
    run NCC with (stride, threshold)
    send best score for that scale
collect results
pick overall best
if refine enabled and stride > 1:
    locally re-scan around best with stride=1
return (x,y,score,scale,found?)
```

## Template Matching (Evaluation Only)

Locate template (small) within frame (large) returning (x,y,score,scale,found?). Score threshold gates action.

## NCC Formula

The normalized cross-correlation score used in template matching:
$$
	ext{score} = \frac{\sum_i F_i T_i - n\,\mu_F\,\mu_T}{n\,\sigma_F\,\sigma_T}
$$
Where:
* $F_i$ – frame pixel value at position $i$ (after any grayscale or channel selection)
* $T_i$ – template pixel value at position $i$
* $n$ – number of contributing pixels
* $\mu_F, \mu_T$ – means of frame patch and template
* $\sigma_F, \sigma_T$ – standard deviations

Normalization removes brightness bias; higher score means better similarity (range roughly -1..+1; negative/inverted ignored here).

## Performance Techniques

1. Integral images for O(1) sums/variance.
2. Stride coarse pass + optional refine.
3. Parallel scale workers; every scale runs, so a frame always gives the same result.
4. Brightness normalized. The template is fully opaque; alpha is not used as a mask.
