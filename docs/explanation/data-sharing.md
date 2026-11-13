---
title: "What Go Fish sends"
summary: "Go Fish sends nothing about you or your fishing. It asks GitHub for the newest version number, and opens a problem report in your browser only when you ask."
order: 2
---

## What stays on your PC

Everything about your fishing. The pictures Go Fish looks at, the sound it listens to, your casts and catches never leave your PC. Go Fish has no account and no sign-in. A problem report you choose to submit carries your settings and recent warnings, as below.

For live fishing, Go Fish sends key and mouse commands over USB to your ESP32. The board sends those inputs back to your PC over Bluetooth.

## What it sends

| To | When | What |
| --- | --- | --- |
| GitHub | At start and every hour | A request for the newest version number. Nothing about you. |
| Buy Me a Coffee, in your browser | Only when you click **Buy me a coffee** | Nothing: Go Fish just opens buymeacoffee.com/socke. What you do there is between you and Buy Me a Coffee. |
| GitHub, in your browser | Only when you click **Report a problem** and then submit the issue | What you write, plus what Go Fish filled in: its version, your Windows version, the game window's title, your settings and its recent warnings. You see all of it before you submit, and the issue is public. |

When you click **Update**, Go Fish downloads the new version from GitHub. See [Update Go Fish](../how-to/update.md).
