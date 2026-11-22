---
title: "How detection works"
summary: "Why the addon marks the bobber, how the box is watched, and what counts as a bite."
order: 1
---

## Why the Bobber Comes from the Addon

Real footage showed the template failing: the bobber is larger than the template, it changes with the camera, and other players' bobbers look the same. The addon already knows which object is ours and marks it, so reading its brackets is simpler and more reliable than recognising the object. It also gives the area to watch, which keeps the bite detector off the rest of the water. The cost is a dependency on the addon being on, with its colour unchanged.

The first addon used, NaowhForever's Glimmer, draws soft brackets blended additively with the scene, so their colour shifts from day to night, and its box sits about 20 px below the bobber. The project's own Bobber Marker draws opaque magenta in exact screen pixels, centred on the bobber, and only while Fishing is channelled. The locator then sees one fixed colour, the click lands on the bobber, the splash falls in the centre of the watched box, and no marker lingers after a reel to be locked on by the next cast.

## Detection Mental Model

The locator scans the game window for the addon's magenta corners and pairs them into a box. There is no score to tune: the colour is fixed, so a box is either there or not.

## Bite Detection Concept

While monitoring, a grayscale ROI stream feeds statistical heuristics (EMA + window diffs). A spike or a jump from the baseline makes the Engine send `EventFishBite()`. If no bite comes, a deadline ends the cast after `max_cast_duration_seconds`.

## Summary Mental Model

Two timer gates (search, cooldown) guarantee forward motion; detection / bite events adjust path opportunistically. Understand states + events + timers to trace any cycle.
