---
title: "FAQ and glossary"
summary: "Short answers to common questions, and the terms the docs use."
order: 11
---

## FAQ

| Question                               | Answer                                                                    |
| -------------------------------------- | ------------------------------------------------------------------------- |
| What do I need to set before a live run? | Select **Auto** or your ESP32's COM port in Settings. The detection defaults usually work. |
| Does Go Fish need Administrator?       | Not to send input: live input comes from the paired ESP32 board. Installing the addon may need write access to the game folder. |
| It misses bites or reels late          | Lower `Threshold` slightly or reduce `Stride`; ensure refine enabled.     |
| False positives on water ripples       | Raise `Threshold`.                                                        |
| Slow over time                         | Reapply settings or reduce scale range; profile for hotspots.             |
| How to reset everything?               | Close app, delete `pixle_bot_config.json` (and optionally logs), restart. |

## Glossary

| Term     | Definition                                     |
| -------- | ---------------------------------------------- |
| Template | Small target image to locate                   |
| Frame    | Full screenshot                                |
| Stride   | Pixel step size in scanning                    |
| NCC      | Normalized cross‑correlation similarity metric |
| ROI      | Region Of Interest after lock                  |
| FSM      | Finite State Machine for fishing loop          |
| RSS      | Resident Set Size (process working set)        |
| EMA      | Exponential Moving Average for bite detection  |

## Concepts

| Term         | Meaning                                           |
| ------------ | ------------------------------------------------- |
| Template     | Small target image to locate in frame             |
| Frame        | Full captured screenshot                          |
| Scale Factor | Resize multiplier applied to template             |
| Stride       | Pixel step size while scanning                    |
| NCC          | Normalized cross‑correlation similarity score     |
| ROI          | Focus patch after initial lock                    |
| FSM          | Finite State Machine governing fishing loop       |
| EMA          | Exponential moving average used in bite detection |
| RSS          | Resident Set Size (process working set)           |
