---
title: "How Go Fish works"
summary: "The addon marks your bobber, Go Fish watches that spot for the splash and clicks. It only looks at the game window and only acts while the game is in front."
order: 1
---

## Seeing the bobber

Finding a small bobber in a busy picture of water is hard. So Go Fish doesn't try: the Bobber Marker addon draws pink brackets around your own bobber, a colour that almost never appears in the game world. Go Fish looks at the game window, finds the brackets and knows exactly where the bobber is.

## Waiting for the bite

Once the bobber is found, Go Fish only watches the small box inside the brackets, about thirty times a second. When a fish bites, the bobber dips and the water splashes, and the box suddenly changes. A moment later the game plays the bite sound. Go Fish waits for both, so other players' bites and spells don't fool it. With the game's sound effects off, the splash alone is the signal.

## One cast at a time

Each cast follows the same steps: cast, find the bobber, watch, reel in, pause, and cast again. If no fish bites in time, or it can't find the bobber, it simply casts again. See [What the Fishing tab shows](../reference/status.md).

## You stay in control

- **Stop button.** Click **Stop** in Go Fish to end the run.
- **Limits.** It stops by itself after the session length, or after a few casts in a row without a catch, so it never fishes on for hours after something went wrong.
- **Only in the game.** Live keys, clicks and pointer movement go through the ESP32 board while the game is the window in front. Switch to another window and it waits.
- **Only what you see.** It looks at the game window and nothing else on your screen.

## No administrator needed for input

Go Fish sends commands to the ESP32 over USB. The board sends keys and mouse actions back to the PC over Bluetooth. This works with normal Windows permissions. Installing the addon may require Administrator rights if the game folder is protected.
