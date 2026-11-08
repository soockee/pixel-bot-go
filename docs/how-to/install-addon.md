---
title: "Install the Bobber Marker addon"
summary: "Link the addon into the game, so the bot can find your bobber, and set the key you cast with."
order: 1
---

The bot finds your bobber by the brackets the Bobber Marker addon draws around it while you fish. Link the addon into the game, so a `git pull` updates it:

```powershell
New-Item -ItemType Junction -Path "C:\Program Files (x86)\World of Warcraft\_forever_\Interface\AddOns\BobberMarker" -Target "$PWD\addon\BobberMarker"
```

Restart the game or type `/reload`. In NaowhForever's Glimmer settings, switch off the Fishing Bobber highlight, because both addons use the same nameplate. Then cast: magenta brackets appear around your bobber and vanish when you reel. If the bobber is not in the middle of the box, move it with `/bobbermarker y <pixels>`; negative moves it down, and `/bobbermarker` shows the current value.

## Set the Fishing cast key

Open **Settings → Keys** in Go Fish. Click the current **Cast key**, then press the same key or mouse side button you use to cast Fishing in game. The app saves it automatically. Use an unmodified letter, top-row digit, F1–F12, Mouse 4, or Mouse 5.
