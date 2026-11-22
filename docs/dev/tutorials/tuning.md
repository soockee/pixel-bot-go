---
title: "Steadier bite detection"
summary: "What keeps bite detection steady."
order: 3
---

## Steadier bite detection

1. Verify a stable marker lock (few `TargetLost` events).
2. Ensure steady lighting; avoid UI overlays near bobber.
3. Keep the game's sound effects on: `splash_sound` then needs the bite sound after the splash, which rules out other players' spells. With them off it reels on the splash alone only after three casts in a row whose splash brought no bite (`unansweredCasts`), or after 3 s when the game makes no sound at all.
