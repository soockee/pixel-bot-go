---
title: "Your first catch"
summary: "Connect the Go Fish ESP32 Probe, choose it in Settings, then set up the game and addon."
order: 1
---

You need a Windows PC, World of Warcraft: Forever with a character who can fish, and a [Go Fish ESP32 Probe](https://github.com/soockee/pixel-bot-go/tree/main/firmware/esp32_ble_probe) with the Go Fish firmware. Pair the board in Windows Bluetooth settings and leave its USB cable plugged in. Live input always goes through this board.

## 1. Start Go Fish

1. Download `pixel-bot-go-windows-amd64.exe` from the [latest release](https://github.com/soockee/pixel-bot-go/releases/latest) and put it in a folder of its own, for example `Documents\Go Fish`.
2. Start the game and log in to your character.
3. Double-click the Go Fish file. If Windows warns about an unknown app, click **More info**, then **Run anyway**.
4. Open **Settings → ESP32 board** and choose **Auto**. If it doesn't find the board, choose its COM port.

## 2. Follow Set up fishing

The first time, Go Fish opens on **Set up fishing**, which walks you through what is missing.

1. **Game window** ticks itself when Go Fish finds the game. If it doesn't, click **Check again**.
2. **Cast key**: keep **F3**, or click the key and press another spare one. Click **Use this key**.
3. **Bobber Marker addon**: click **Install addon**, then type `/reload` in the game's chat and press Enter.

Then the picture of the game appears and you are ready. You don't bind anything in the game yourself: the addon puts Fishing on your cast key. If something goes wrong here, see [Install the addon](../how-to/install-addon.md).

## 3. Fish

1. In the game, stand at the water and face it.
2. In Go Fish, click **Start fishing**, then click into the game.

Go Fish casts, waits for the splash, reels in and casts again. The Fishing tab counts your catches.

## Stop

Click **Stop** in Go Fish to end the run. It also stops by itself after an hour, or after five casts in a row without a bite.

Next, if you like: [change keys and limits](../how-to/settings.md).
