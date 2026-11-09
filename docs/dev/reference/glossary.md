---
title: "FAQ and glossary"
summary: "Short answers to common questions, and the terms the docs use."
order: 11
---

## FAQ

| Question                               | Answer                                                                    |
| -------------------------------------- | ------------------------------------------------------------------------- |
| Do I need to tweak settings first run? | No, defaults usually work.                                                |
| Why run as Administrator?              | Only when the game runs elevated: UIPI blocks input into a higher-integrity process. |
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
