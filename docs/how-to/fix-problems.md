---
title: "Fix common problems"
summary: "What to do when Go Fish won't start, can't find the bobber, misses bites or stops early."
order: 6
---

The status bar at the bottom of Go Fish usually says what is wrong. These are the common cases.

## Start fishing is greyed out

- **Not signed in, or no role.** Sign in, and ask the owner for a role. See [Sign in](sign-in.md).
- **No game window.** Start the game, then pick **World of Warcraft** on the Fishing tab.

## It casts, but never finds the bobber

- Check that the pink brackets appear around your bobber in the game. If not, see [Install the Bobber Marker addon](install-addon.md).
- Keep the bobber visible: no windows, bags or chat over it, and don't turn the camera while fishing.
- After five casts without finding anything it stops; that's the **Casts without a bite** limit.

## It sits on "Focus"

Go Fish only fishes while the game is the window in front. Click into the game.

## Nothing happens in the game

- Start Go Fish with **Run as administrator**. Without it Windows blocks its key presses and clicks.
- Check the **Cast key** under Settings matches your Fishing key in the game.

## It reels too early, or misses bites

- Move the brackets so the bobber sits in the middle; see [Addon commands](../reference/addon-commands.md).
- Fish where the water is calm and the bobber is easy to see, away from other players' bobbers.

## The stop key doesn't work

Another program already uses that key. Pick a different **Stop key** under Settings and restart Go Fish.

## Still stuck

Open an issue on [GitHub](https://github.com/soockee/pixel-bot-go/issues) and attach `pixel_bot_logs.json` from the Go Fish folder; it holds what Go Fish did and your email if you are signed in, but no passwords or sign-in tokens.
