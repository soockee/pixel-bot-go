---
title: "Troubleshoot"
summary: "What to check when the bot finds nothing, triggers too early or runs slowly."
order: 9
---

## Symptoms and fixes

| Symptom            | Action                                            |
| ------------------ | ------------------------------------------------- |
| No matches         | Check Settings → Addon says up to date, then `/reload` in the game |
| Slow scan          | Use a smaller game window                         |
| Memory pressure    | Profile the heap                                  |

## Next Steps When Stuck

Investige profile `.pb.gz` files; Compare both baseline and later snapshots plus goroutine counts. use top command to identify hotspots or retention paths

## Quick Setup Recap

1. Install Bobber Marker from Settings → Addon.
2. Run capture.
3. Enable debug only when profiling (minor overhead).

## Operating Tips

* Connect the paired ESP32 over USB and select **Auto** or its COM port in Settings.
* Keep the game window focused. Go Fish waits while another window is active.
* Play in a smaller window for faster searches; watching for a bite captures only the bobber's box either way.
* Pause before changing settings; apply then resume.

## FAQ

| Question                 | Answer                                               |
| ------------------------ | ---------------------------------------------------- |
| Cursor doesn't move      | Keep the game focused and the pointer inside it. Right-click reeling needs firmware that supports pointer movement. |
| Never finds bobber       | Bobber Marker must be on and up to date; its brackets show only while Fishing is channelled. |
| Wants to fish forever    | Check `max_cast_duration_seconds` isn't too large.   |
| Reset config             | Delete `pixle_bot_config.json` while app closed.     |
