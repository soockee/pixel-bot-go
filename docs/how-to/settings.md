---
title: "Change keys and limits"
summary: "Set the key you cast with and how long Go Fish fishes."
order: 3
---

Open the **Settings** tab. Every change saves at once. Settings are locked while Go Fish is fishing; click **Stop** first.

## Keys

- **Cast key**: click it, then press a spare key: a letter, a number on the top row, F1 to F12, or a mouse side button, without Shift, Ctrl or Alt. The addon puts Fishing on that key for you. After a change, click **Update addon** on the Fishing tab and type `/reload` in the game.

- **Reel method**: Go Fish right-clicks the bobber by default. To reel with a key instead, choose **Interact key** and pick a spare key next to it; the addon sets it up after an **Update** and `/reload`. If your bobber lands too far away for the key to reach, go back to **Right click**.

## ESP32 board

If you have an ESP32 board with the [Go Fish firmware](https://github.com/soockee/pixel-bot-go/tree/main/firmware/esp32_ble_probe), Go Fish can press keys and click through it instead of through Windows. Pair **Go Fish ESP32 Probe** in the Windows Bluetooth settings, keep its USB cable plugged in, and pick **Auto** under **ESP32 board**. If it isn't found, pick its COM port instead, for example `COM3`. Pick **Off** to go back.

## Scheduled breaks

1. Under **Scheduled breaks**, switch on **Take breaks**. Set the session and break ranges, and pick a spare key as the **Logout key**.
2. Click **Update addon** on the Fishing tab and type `/reload` in the game. The addon makes a **Logout** macro and puts it on that key.
3. Click **Start fishing** while logged in to your character, with the game in front.

Each session and each break gets a new random length. At the end of a break Go Fish presses Enter to log back in and fishes on. Keep the game in front while it switches. If it can't tell where you are within 90 seconds, it gives up. Click **Stop** in Go Fish to end the whole schedule.

## Run

- **Session length**: Go Fish stops at the first cast after this time.
- **Casts without a bite**: Go Fish stops after this many casts in a row without a catch, for example when you have run out of bait or moved away from the water.
- **Pause after a catch**: how long it waits before casting again. **Quick**, the default, casts again within about a second.
- **Cast length**: if nothing bites within this time, it casts again.

## Move the overlay

While Go Fish fishes, a small pill sits over the top of the game and shows what it is doing and how many fish it has caught. If it is in your way, move it:

1. Start the game, so Go Fish has a window to put it on. Don't start fishing.
2. In **Settings**, under **Overlay**, click **Move overlay**. The pill appears over the game with a dashed outline.
3. Drag it where you want it in the game.
4. Click **Done** in Go Fish.

The place stays when you resize the game window. Once you are done, the pill lets every click through to the game again.

## Appearance

- **Theme**: pick one of six looks. Each changes more than colours: the words on the buttons and tabs, the layout, the window and taskbar icon, and what happens when you land a fish.
  - **Graphite**: the plain dark panel. Quiet.
  - **Tavern Ledger**: parchment and brass like a quest log. Catches rain gold coins.
  - **Arcane Grimoire**: violet runes turn around the picture and speed up while it watches for a bite.
  - **Frostwater**: light and calm with falling snow. While it watches, everything but the water fades back.
  - **Felfire Forge**: black and green, loud and sharp, with the controls on the left.
  - **Tidecaller**: bright beach colours, waves and bubbles, with the tabs at the bottom.
- **Theme sounds**: turn the catch sound off or on.

All settings with their defaults are listed under [Settings](../reference/settings.md).
