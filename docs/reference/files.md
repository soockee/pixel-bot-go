---
title: "Files on your PC"
summary: "What Go Fish keeps next to its exe, and what is safe to delete."
order: 4
---

Go Fish keeps everything in the folder it runs from.

| File | Holds | Safe to delete? |
| --- | --- | --- |
| `pixle_bot_config.json` | Your settings | Yes, Go Fish starts with the defaults |
| `pixel_bot_logs.json` | What Go Fish did in its last run; replaced at each start | Yes |
| `pixel_bot_account.bin` | Your saved sign-in, encrypted for your Windows user | Yes, you sign in again |
| `datasets\` | Your recordings, if you made any | Yes, unless you still want to share them |
| `pixel-bot-go.exe.old` | The previous version, right after an update | Removed by itself at the next start |

Nothing else is written to your PC, apart from the addon's own setting inside the game's folder.
