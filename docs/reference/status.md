---
title: "What the Fishing tab shows"
summary: "The steps of a cast, the counters and the messages you see while Go Fish fishes."
order: 2
---

## Steps

Go Fish goes round these steps; the Fishing tab highlights the current one.

| Step | Meaning |
| --- | --- |
| Focus | Waiting for the game to be the window in front. Live input goes through the ESP32, so Go Fish waits while another window is active. |
| Search | Looking for the pink brackets around the bobber. |
| Watch | Watching the box for the splash of a bite. |
| Reel | A fish bit; clicking the bobber. |
| Cool down | The short pause after a catch. |
| Cast | Pressing your cast key. |

## Counters

- **Catches this run**: fish reeled in since you pressed Start, and how many casts that took.
- **Caught, got away, not hooked**: how the reels ended, as the game told Bobber Marker. "Not hooked" means Go Fish reeled with no fish on the hook. Shown once Bobber Marker 1.5.0 or later is installed.
- **Session**: how long this run has gone, against the session length.
- **Casts without a bite**: casts in a row without a catch, against the limit that stops the run.

## Thank-you note

When Go Fish has caught 100 fish for you in total, and again at 500, 1,000, 2,500, 5,000 and every 5,000 after, a short note under the counters says how many fish and how many hours of fishing that was, and offers to say thanks. It only shows after a run that went well (at least one catch, few reels with no fish hooked), never during a dry run. **Not now** hides it until the next milestone; **I already support it** and **Don't ask again** hide it for good, and only the small link in the status bar stays. The button's wording follows the theme. Go Fish keeps the counts on your PC only.

## Picture

The large picture is what Go Fish sees of the game window. While it watches for a bite, the picture zooms in on your bobber and the water around it, live; the rest of the window dims.

## Status bar

The bottom line shows the latest message, such as why a start was refused or how far an update has got, a **Buy me a coffee** link, and the version you have.
