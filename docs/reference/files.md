---
title: "Generated files"
summary: "The files Go Fish writes beside itself, and which are safe to delete."
order: 4
---

| File                    | Purpose                                  | Notes                                                   |
| ----------------------- | ---------------------------------------- | ------------------------------------------------------- |
| `pixle_bot_config.json` | Persist user settings between runs       | Safe to edit while app closed; delete to reset defaults |
| `pixel_bot_logs.json`   | Structured log of events & state changes | Can be deleted; recreated automatically                 |
| `<exe>.old`, `<exe>.new` | The build an update replaced, and one being downloaded | `.old` is removed on the next start, `.new` when the download fails |
| `pixel_bot_account.bin` | Google refresh token of the signed-in account | DPAPI-encrypted for this Windows user; deleted on Sign out; never commit |
