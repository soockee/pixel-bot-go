---
title: "How Go Fish works"
summary: "The addon marks your bobber, Go Fish watches that spot for the splash and clicks. It only looks at the game window and only acts while the game is in front."
order: 1
---

## Seeing the bobber

Finding a small bobber in a busy picture of water is hard. So Go Fish doesn't try: the Bobber Marker addon draws pink brackets around your own bobber, a colour that almost never appears in the game world. Go Fish looks at the game window, finds the brackets and knows exactly where the bobber is.

## Waiting for the bite

Once the bobber is found, Go Fish only watches the small box inside the brackets, about thirty times a second. When a fish bites, the bobber dips and the water splashes, and the box suddenly changes. That is the signal to click.

## One cast at a time

Each cast follows the same steps: cast, find the bobber, watch, click on a bite, pause, and cast again. If no fish bites in time, Go Fish casts again. If it can't find the bobber, it tries again on the next cast. See [What the Fishing tab shows](../reference/status.md).

## You stay in control

- **Stop key.** F10 stops Go Fish instantly, from anywhere.
- **Limits.** It stops by itself after the session length, or after a few casts in a row without a catch, so it never fishes on for hours after something went wrong.
- **Only in the game.** It only presses keys and clicks while the game is the window in front. Switch to another window and it waits.
- **Only what you see.** It looks at the game window and nothing else on your screen.

## Why administrator

Windows doesn't let normal programs send key presses and clicks into a game that runs with more rights. Running Go Fish as administrator gives it the same rights as the game, nothing more.
