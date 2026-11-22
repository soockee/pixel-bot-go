---
title: "Files on your PC"
summary: "What Go Fish keeps in its folder, in your user folders and in the game, and what is safe to delete."
order: 4
---

## In the Go Fish folder

Go Fish keeps its settings and logs in the folder you start it from, usually the folder with the file you downloaded.

| File | Holds | Safe to delete? |
| --- | --- | --- |
| `pixle_bot_config.json` | Your settings, where the overlay sits, and your catch totals for the thank-you note | Yes, Go Fish starts with the defaults and counts from zero |
| `pixel_bot_logs.json` | What Go Fish did since it started. A log that grows past 20 MB moves on to a fresh one | Yes |
| `pixel_bot_logs.1.json`, `pixel_bot_logs.2.json` | The logs of the two starts before, so a problem report still has them after a restart | Yes |
| `go-fish-report-` with a date and time, ending in `.zip` | A report file you saved under **Settings → Report file**: the logs, your settings and a summary | Yes, once you have shared it |
| `datasets\` | Recordings, made only in developer mode | Yes, unless you still want to share them |
| The Go Fish file ending in `.old` | The previous version, right after an update | Removed by itself at the next start |

## In your user folders

| Where | Holds | Safe to delete? |
| --- | --- | --- |
| `%TEMP%\pixel-bot-go-fish-` and a number | A copy of Go Fish named after a random fish, such as `mackerel.exe`. Each start runs from a new copy, so the window and the process carry that name | Yes, while Go Fish is closed. Copies older than a day are removed at the next start |
| `%APPDATA%\` and the name of the Go Fish file | The window's browser data | Yes, while Go Fish is closed |

## In the game

When you click **Install addon** or **Update**, Go Fish writes the Bobber Marker addon into the game's `Interface\AddOns\BobberMarker` folder, including `Keys.lua` with your Cast key and, when your settings use them, your Interact and Logout keys. It touches no other addon. Delete that folder to remove the addon.

The addon itself changes a few game settings and keeps its own setting with your other addon settings; see [Addon commands](addon-commands.md).
