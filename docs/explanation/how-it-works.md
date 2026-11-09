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

Each cast follows the same steps: cast, find the bobber, watch, reel in, pause, and cast again. If no fish bites in time, or it can't find the bobber, it simply casts again. See [What the Fishing tab shows](../reference/status.md).

## You stay in control

- **Stop button.** Click **Stop** in Go Fish to end the run.
- **Limits.** It stops by itself after the session length, or after a few casts in a row without a catch, so it never fishes on for hours after something went wrong.
- **Only in the game.** It only presses keys and clicks while the game is the window in front. Switch to another window and it waits.
- **Only what you see.** It looks at the game window and nothing else on your screen.

## No administrator needed

Go Fish runs with your normal Windows rights, like the game. Only if you start the game as administrator does Go Fish need the same, because Windows doesn't let a program press keys in a game that runs with more rights than it has.
