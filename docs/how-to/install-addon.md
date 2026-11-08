---
title: "Install the Bobber Marker addon"
summary: "Copy the addon into the game's AddOns folder; it draws the pink brackets Go Fish looks for while you fish."
order: 1
---

The Bobber Marker addon draws pink brackets around your own bobber while you fish, and nothing else. Without it Go Fish can't find the bobber.

1. On the [Go Fish GitHub page](https://github.com/soockee/pixel-bot-go), click **Code → Download ZIP** and unpack it.
2. Copy the folder `addon\BobberMarker` into the game's AddOns folder:
   `World of Warcraft\_forever_\Interface\AddOns\`
   You should end up with `...\Interface\AddOns\BobberMarker\BobberMarker.toc`.
3. Restart the game, or type `/reload` if it is running.
4. On the character screen, check that **Bobber Marker** is ticked under **AddOns**.

## Check that it works

Cast Fishing. Pink brackets appear around your bobber and disappear when you reel in.

**No brackets?** If you use NaowhForever, open its Glimmer settings and switch off the **Fishing Bobber** highlight; both addons use the same marker spot above the bobber.

**Bobber not in the middle of the brackets?** Move the box with `/bobbermarker y <pixels>`. Negative numbers move it down. `/bobbermarker` shows the current value. See [Addon commands](../reference/addon-commands.md).

## What the addon changes in your game

To put a marker on the bobber, the addon uses the game's soft targeting. The first time you fish it switches on **Enable Interact Key** and lets world objects get a nameplate, and those settings stay as you leave them afterwards. The bobber's nameplate is only on while you fish; then it goes back to how you had it. The addon sends nothing anywhere.

## Update the addon

Download the ZIP again and replace the `BobberMarker` folder.
