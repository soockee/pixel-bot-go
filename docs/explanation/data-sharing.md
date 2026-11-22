---
title: "What Go Fish sends"
summary: "Go Fish sends nothing about you or your fishing. It asks GitHub for the newest version number, and opens a problem report in your browser only when you ask."
order: 2
---

## What stays on your PC

Everything about your fishing. The pictures Go Fish looks at, the sound it listens to, your casts and catches never leave your PC. Go Fish has no account and no sign-in. A problem report you choose to submit carries your settings, a summary of your last runs and recent warnings, as below. Go Fish keeps its own log of the last three starts in its folder.

For live fishing, Go Fish sends key and mouse commands over USB to your ESP32. The board sends those inputs back to your PC over Bluetooth.

## What it sends

| To | When | What |
| --- | --- | --- |
| GitHub | At start and every hour | A request for the newest version number. Nothing about you. |
| Buy Me a Coffee, in your browser | Only when you click **Buy me a coffee** | Nothing: Go Fish just opens buymeacoffee.com/socke. What you do there is between you and Buy Me a Coffee. |
| GitHub, in your browser | Only when you click **Report a problem** and then submit the issue | What you write, plus what Go Fish filled in: its version, your Windows version, the game window's title, a summary of your last runs from its log (casts, bites, how they ended, why a run stopped), your settings and its recent warnings. You see all of it before you submit, and the issue is public. |
| Wherever you drag it | Only when you click **Save report file** and drag the file somewhere | Go Fish saves the file in its folder and shows it to you; it sends nothing itself. The file holds the logs of the last three starts, your settings and the report text above. The logs name your game folder and may hold your Windows user name in a path. |

A few buttons open a page in your browser and send nothing themselves: **What's new** opens the release notes on GitHub, and **What to do** after a stop opens this guide.

When you click **Update**, Go Fish downloads the new version from GitHub. See [Update Go Fish](../how-to/update.md).
