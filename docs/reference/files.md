---
title: "Files on your PC"
summary: "What Go Fish keeps in its folder, and what is safe to delete."
order: 4
---

Go Fish keeps everything in the folder it runs from.

| File | Holds | Safe to delete? |
| --- | --- | --- |
| `pixle_bot_config.json` | Your settings | Yes, Go Fish starts with the defaults |
| `pixel_bot_logs.json` | What Go Fish did in its last run; replaced at each start | Yes |
| `datasets\` | Your recordings, if you made any | Yes, unless you still want to share them |
| A file ending in `.old` | The previous version of Go Fish, right after an update | Removed by itself at the next start |

Apart from these, Go Fish writes only the Bobber Marker addon into the game's `Interface\AddOns\BobberMarker` folder, when you click **Install**. It touches no other addon.
