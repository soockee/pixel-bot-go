---
title: "Install the addon"
summary: "Go Fish installs its Bobber Marker addon for you: click Install, then type /reload in the game."
order: 1
---

Go Fish needs its small **Bobber Marker** addon in the game. It draws pink brackets around your bobber so Go Fish can find it, and it sets up your cast key. You don't download it: Go Fish brings it along.

1. Start the game first, then Go Fish.
2. On the **Fishing** tab, **Set up fishing** first asks for your cast key. Keep it or pick another and click **Use this key**, then click **Install addon**.
3. In the game, type `/reload` and press Enter.

To check it, cast Fishing in the game. Pink brackets appear around your bobber.

## When Go Fish says Update

After Go Fish updates itself, or when you change a key in Settings, the addon needs to be updated too. **Set up fishing** comes back with an **Update addon** button. Click it and type `/reload` in the game again.

You can also install and update the addon under **Settings → Addon**.

## If it doesn't work

**"Start the game, so Go Fish can find its folder"**: Go Fish finds the game through its window. Start the game, then click **Check again**.

**"Windows refused to write"**: Close Go Fish and start it again with **Run as administrator**, then click **Install**.

**No pink brackets when you cast**: On the character screen, click **AddOns** and make sure **Bobber Marker** is ticked. If you use NaowhForever, switch off its **Fishing Bobber** highlight; the two get in each other's way.

**The bobber isn't in the middle of the brackets**: Type `/bobbermarker y -60` in the game to move the box down a little, or `-20` to move it up. See [Addon commands](../reference/addon-commands.md).

**Your cast key does nothing**: Type `/bobbermarker` in the game; it says what went wrong. You can always bind the key yourself in the game's **Key Bindings** to the **Fishing** macro.

## What the addon changes in your game

- It makes a macro called **Fishing** and puts it on your cast key. Whatever was on that key before is replaced.
- If you reel with the Interact key, it puts **Interact With Target** on that key.
- If you take breaks or use Change spot, it makes a macro called **Logout** and puts it on your Logout key.
- It switches on the game's **Interact Key** option and nameplates for objects, which it needs to find the bobber.

It never changes anything during a fight, and it sends nothing anywhere.
