---
title: "Troubleshoot"
summary: "What to check when the bot finds nothing, triggers too early or runs slowly."
order: 9
---

## Symptoms and fixes

| Symptom            | Action                                            |
| ------------------ | ------------------------------------------------- |
| No matches         | Lower `Threshold`, verify scale range covers size |
| Many false matches | Raise `Threshold`, keep refine enabled            |
| Slow scan          | Narrow the scales, or use a smaller game window   |
| Memory pressure    | Reduce scale count; profile heap                  |

## Next Steps When Stuck

Investige profile `.pb.gz` files; Compare both baseline and later snapshots plus goroutine counts. use top command to identify hotspots or retention paths

## Quick Setup Recap

1. Configure detection scales conservatively.
2. Run capture.
3. Observe timings and adjust stride/threshold.
4. Enable debug only when profiling (minor overhead).

## Operating Tips

* Run as Administrator only when the game runs elevated; UIPI drops input into a higher-integrity process without an error.
* Keep game window focused; alt-tab can delay key events.
* Play in a smaller window for faster searches; watching for a bite captures only the bobber's box either way.
* Pause before changing settings; apply then resume.
* Use dark mode if the UI is distracting in low light.

## FAQ

| Question                 | Answer                                               |
| ------------------------ | ---------------------------------------------------- |
| Cursor doesn't move      | Game elevated? Match it; confirm cast key binding.   |
| Never finds bobber       | Lower `Threshold`; ensure scale range brackets size. |
| Triggers on wrong pixels | Raise `Threshold`; refine ON. |
| Wants to fish forever    | Check `max_cast_duration_seconds` isn't too large.   |
| Reset config             | Delete `pixle_bot_config.json` while app closed.     |
