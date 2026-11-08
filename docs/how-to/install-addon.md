---
title: "Install the Bobber Marker addon"
summary: "Go Fish installs the addon into the game for you with one click, and tells you when it needs an update."
order: 1
---

The Bobber Marker addon draws pink brackets around your own bobber while you fish, and nothing else. Without it Go Fish can't find the bobber. Go Fish carries the addon and installs it for you.

1. Start World of Warcraft: Forever, so Go Fish can find the game's folder.
2. In Go Fish, open **Settings** and find **Addon**. It shows the game's AddOns folder.
3. Click **Install**.
4. In the game, type `/reload`, or restart it.

When **Addon** says **Up to date**, you're done. After a Go Fish update brings a newer addon, the button says **Update**; click it and `/reload` again.

**"Start World of Warcraft: Forever"?** Go Fish finds the game through its window, or in the usual install folder `C:\Program Files (x86)\World of Warcraft\_forever_`. Start the game and click **Check again**.

**"Windows refused to write"?** The game folder needs administrator rights. Start Go Fish with **Run as administrator** and click **Install** again.

## Check that it works

Cast Fishing. Pink brackets appear around your bobber and disappear when you reel in.

**No brackets?** On the character screen, check that **Bobber Marker** is ticked under **AddOns**. If you use NaowhForever, open its Glimmer settings and switch off the **Fishing Bobber** highlight; both addons use the same marker spot above the bobber.

**Bobber not in the middle of the brackets?** Move the box with `/bobbermarker y <pixels>`. Negative numbers move it down. `/bobbermarker` shows the current value. See [Addon commands](../reference/addon-commands.md).

## What the addon changes in your game

To put a marker on the bobber, the addon uses the game's soft targeting. The first time you fish it switches on **Enable Interact Key** and lets world objects get a nameplate, and those settings stay as you leave them afterwards. The bobber's nameplate is only on while you fish; then it goes back to how you had it. The addon sends nothing anywhere.

## Your Cast and Interact keys

When Go Fish installs the addon it also tells it your **Cast key** and, if you reel with the Interact key, your **Interact key** from Settings. Each time you log in, and when a fight ends, the addon sets them up in the game, so you don't have to:

- It creates an account-wide macro called **GoFish Cast** that casts Fishing, and binds your Cast key to it.
- It binds your Interact key to the game's **Interact With Target** action.

It says in the chat what it changed. It never changes keys in combat, and it replaces whatever your Cast or Interact key was bound to before. If you change either key in Settings, **Addon** says **Outdated**: click **Update** and `/reload`.

**Chat says it could not bind a key?** Open the game's **Key Bindings** and bind the key yourself: to the **GoFish Cast** macro, or to **Interact With Target**.

## Install it by hand

If Go Fish can't reach the game folder, download the [Go Fish source as a ZIP](https://github.com/soockee/pixel-bot-go/archive/refs/heads/main.zip), unpack it, and copy `addon\BobberMarker` into `World of Warcraft\_forever_\Interface\AddOns\`. A hand install doesn't know your keys, so bind them yourself as above, or add a `Keys.lua` file next to `BobberMarker.lua` that holds `BobberMarkerKeys = { cast = "F3", interact = "F" };` with your keys.
