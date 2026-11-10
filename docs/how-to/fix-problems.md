---
title: "Fix common problems"
summary: "What to do when Go Fish won't start, can't find the bobber, misses bites or stops early."
order: 6
---

The status bar at the bottom of Go Fish usually says what is wrong. These are the common cases.

## Start fishing is greyed out

Something in **Set up fishing** is still missing. Usually the game isn't open, or the addon needs an install or update. Follow the list; see [Install the addon](install-addon.md).

## It stopped by itself

When Go Fish stops without you pressing Stop, the status bar says **Stopped:** and why:

- **N casts in a row got no bite.** It cast the number of times you set under **Casts without a bite** and none got a bite. Check that the pink brackets show around your bobber, and that you fish where fish bite.
- **The mouse was moved.** Go Fish parks the pointer on the bobber while it waits, and stops when you move the mouse so that it never fights you for it. Leave the mouse alone while it fishes, and click **Start fishing** again.
- **The pointer was moved away, or the game was not in front.** The same, when the game lost the focus at the wrong moment.
- **The N minute session limit was reached.** The limit under **Session** in Settings ended the run.
- Other messages name the setting to check, such as the Cast or Interact key.

## It casts, but never finds the bobber

- Check that the pink brackets appear around your bobber in the game. If not, open **Settings → Addon** in Go Fish: it should say **Up to date**. See [Install the addon](install-addon.md).
- Keep the bobber visible: no windows, bags or chat over it, and don't turn the camera while fishing.
- After five casts without finding anything it stops; that's the **Casts without a bite** limit.

## It sits on "Focus"

Go Fish only fishes while the game is the window in front. Click into the game. With [Fish in the background](settings.md#fish-in-the-background) on, it waits here only while the game is minimized.

## Nothing happens in the game

- If you start the game or Battle.net as administrator, Windows blocks Go Fish's key presses and clicks. Start the game normally, or close Go Fish and start it with **Run as administrator** too.
- If you changed the **Cast key**, click **Update addon** in **Set up fishing** and type `/reload` in the game.

## It reels too early, or misses bites

- Move the brackets so the bobber sits in the middle; see [Addon commands](../reference/addon-commands.md).
- Fish where the water is calm and the bobber is easy to see, away from other players' bobbers.
- With the game's sound effects off, Go Fish needs three casts without a reel before it reels on the splash alone. Keep sound effects on: then it waits for the bite sound, and other players' spells and ripples do not fool it.


## Still stuck

Click **Report a problem** at the top right of Go Fish. Your browser opens a new issue on [GitHub](https://github.com/soockee/pixel-bot-go/issues) with the Go Fish version, your Windows version, your settings and Go Fish's recent warnings already filled in. Describe what happened, pick where it happened, and submit. You need a GitHub account to submit, and nothing is sent before you do.

If you can, drag the file `pixel_bot_logs.json` from the Go Fish folder into the issue. It holds what Go Fish did, but no passwords.

## It works again

Glad it helped. Go Fish is free and made by one person in their spare time; if it saves you time, you can [buy me a coffee](https://buymeacoffee.com/socke).
