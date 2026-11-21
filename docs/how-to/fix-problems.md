---
title: "Fix common problems"
summary: "What to do when Go Fish won't start, can't find the bobber, misses bites or stops early."
order: 6
---

The status bar at the bottom of Go Fish usually says what is wrong. These are the common cases.

## Start fishing is greyed out

Something in **Set up fishing** is still missing. Usually the game isn't open, the ESP32 board isn't found, or the addon needs an install or update. Follow the list; see [Install the addon](install-addon.md).

## It stopped by itself

When Go Fish stops without you pressing Stop, the status bar says **Stopped:** and why:

- **N casts in a row got no bite.** It cast the number of times you set under **Casts without a bite** and none got a bite. Check that the pink brackets show around your bobber, and that you fish where fish bite.
- **The mouse was moved.** Go Fish parks the pointer on the bobber while it waits, and stops when you move the mouse so that it never fights you for it. Leave the mouse alone while it fishes, and click **Start fishing** again.
- **The game was not in front.** Another window took the focus while Go Fish moved the pointer. Keep the game in front while it fishes.
- **The pointer path left the game window.** Keep the bobber away from the edges of the game window.
- **The ESP32 could not put the pointer on the target.** The board's pointer moves did not arrive in time, usually over a weak Bluetooth link. Move the board closer to the PC, and turn off the board's Wi-Fi status page while fishing.
- **The N minute session limit was reached.** The limit under **Session** in Settings ended the run.
- Other messages name the setting to check, such as the Cast or Interact key.

## It casts, but never finds the bobber

- Check that the pink brackets appear around your bobber in the game. If not, open **Settings → Addon** in Go Fish: it should say **Up to date**. See [Install the addon](install-addon.md).
- Keep the bobber visible: no windows, bags or chat over it, and don't turn the camera while fishing.
- After five casts without finding anything it stops; that's the **Casts without a bite** limit.

## It sits on "Focus"

Go Fish sends live input through the ESP32 to the window in front. Click into the game to continue.

## Nothing happens in the game

- Check that the Go Fish ESP32 Probe is paired in Windows Bluetooth settings, plugged in over USB, and selected under **Settings → ESP32 board**. If **Auto** doesn't find it, choose its COM port.
- Click **Test** next to **Settings → ESP32 board**. If checks over USB go missing, try another cable or USB port. If the board answers over USB but its key or pointer does not reach Windows, remove **Go Fish ESP32 Probe** in Bluetooth settings and pair it again.
- Check that the board has the current Go Fish firmware and that no other program has its COM port open.
- If you changed a key, click **Update addon** in **Set up fishing** and type `/reload` in the game.

## It reels too early, or misses bites

- Move the brackets so the bobber sits in the middle; see [Addon commands](../reference/addon-commands.md).
- Fish where the water is calm and the bobber is easy to see, away from other players' bobbers.
- With the game's sound effects off, Go Fish needs three casts without a reel before it reels on the splash alone. Keep sound effects on: then it waits for the bite sound, and other players' spells and ripples do not fool it.

## It reels in, but the fish stays in the loot window

Turn on **Auto Loot** in the game (Interface options, **Controls**). Go Fish clicks the bobber and leaves the looting to the game.

## Many fish get away

"Your fish got away" comes from the game, not from Go Fish: it happens often when your fishing skill is low for the water you fish in. Fish where your skill is high enough.


## Still stuck

Click **Report a problem** at the top right of Go Fish. Your browser opens a new issue on [GitHub](https://github.com/soockee/pixel-bot-go/issues) with the Go Fish version, your Windows version, a summary of your last runs, your settings and Go Fish's recent warnings already filled in. Describe what happened, pick where it happened, and submit. You need a GitHub account to submit, and nothing is sent before you do. Go Fish keeps the logs of its last three starts, so the report still holds the problem after you restarted it.

If you are asked for the log, open **Settings**, click **Save report file** under **Report file**, and drag the file Go Fish shows you into the issue. It holds the last logs and your settings, but no passwords.

## It works again

Glad it helped. Go Fish is free and made by one person in their spare time; if it saves you time, you can [buy me a coffee](https://buymeacoffee.com/socke).
