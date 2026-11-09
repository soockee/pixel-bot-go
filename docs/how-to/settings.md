---
title: "Change keys and limits"
summary: "Set the key you cast with, the stop key, and how long and how stubbornly Go Fish fishes."
order: 3
---

Open the **Settings** tab. Every change saves at once. Settings are locked while Go Fish is fishing; click **Stop** first.

## Keys

- **Cast key**: click it, then press the key or mouse side button you cast Fishing with in the game. Use a letter, a number on the top row, F1 to F12, Mouse 4 or Mouse 5, without Shift, Ctrl or Alt.
- **Stop key**: the key that stops Go Fish from anywhere, even while the game is in front. Pick one you don't use in the game. It takes effect after you restart Go Fish.

To reel with the game's native Interact binding, enable **Interact Key** in the game and try reeling manually at your fishing spot. In Go Fish, set **Interact key** to the same binding, then choose **Interact key** under **Reel method**. Go Fish still uses Bobber Marker to identify the bobber and checks for the marker again before pressing the key. If your cast is beyond the game's interaction range, choose **Right click**.

To use an ESP32, flash the [ESP32 firmware](https://github.com/soockee/pixel-bot-go/tree/main/firmware/esp32_ble_probe), pair **Go Fish ESP32 Probe** in Windows Bluetooth settings, and keep its USB cable connected. Cast, Interact and the other keys can be letters, digits, F1–F12, or the mouse side buttons Mouse 4 and Mouse 5. The **Right click** reel method also works: the board moves the cursor and clicks. Then pick **Auto** under **ESP32 board**. Go Fish asks each USB serial port for the Go Fish firmware and uses the one that answers; if you have several boards, or it finds none, pick the board's COM port such as `COM3` instead. A board that was just plugged in or reset can take a few seconds to find your PC over Bluetooth, and Go Fish waits for it. A failed tap or click stops the run. Pick **Off** to use Windows input again.

## Scheduled breaks

1. Bind a WoW macro containing `/logout` to a spare key, and check that Enter enters the world from character selection.
2. Keep Bobber Marker up to date (Settings → Addon). Its small coloured blocks in the top-left corner tell Go Fish you are in the world; on the character screen there are none.
3. Under **Scheduled breaks**, switch on **Take breaks**, set the session and break ranges and the **Logout key**. Click **Start fishing** while logged in to your character, with the game in front.

Go Fish picks a new session length and a new break length for each cycle. The break starts once it sees the character screen. At the end it presses Enter and waits until the addon shows up again before it fishes. Keep the game in front for the switch. If it can't confirm a screen within 90 seconds, it cancels the schedule. **Stop** and the Stop key cancel the whole schedule, including a pending login.

## Run

- **Session length**: Go Fish stops at the first cast after this time.
- **Casts without a bite**: Go Fish stops after this many casts in a row without a catch, for example when you have run out of bait or moved away from the water.
- **Pause after a catch**: how long it waits before casting again.
- **Cast length**: if nothing bites within this time, it casts again.

## Move the overlay

While Go Fish fishes, a small pill sits over the top of the game and shows what it is doing, how many fish it has caught and which key stops it. If it is in your way, move it:

1. Start the game, so Go Fish has a window to put it on. Don't start fishing.
2. In **Settings**, under **Overlay**, click **Move overlay**. The pill appears over the game with a dashed outline.
3. Drag it where you want it in the game.
4. Click **Done** in Go Fish.

Go Fish keeps the place, as a share of the game window, so it holds when you change the window size or resolution. The pill can't be dragged out of the window. While you move it the pill takes the mouse, but only the pill does: the rest of the game stays clickable. Once you are done, or when you start fishing, it takes no clicks again. It never shows up in the picture Go Fish looks at.

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
