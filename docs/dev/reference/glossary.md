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
| It never finds the bobber              | Settings → Addon must say up to date; type `/reload` in the game after an update. |
| Slow over time                         | Profile for hotspots.                                                     |
| How to reset everything?               | Close app, delete `pixle_bot_config.json` (and optionally logs), restart. |

## Glossary

| Term     | Definition                                     |
| -------- | ---------------------------------------------- |
| Frame    | Full screenshot                                |
| ROI      | Region Of Interest after lock                  |
| FSM      | Finite State Machine for fishing loop          |
| RSS      | Resident Set Size (process working set)        |
| EMA      | Exponential Moving Average for bite detection  |

## Concepts

| Term         | Meaning                                           |
| ------------ | ------------------------------------------------- |
| Frame        | Full captured screenshot                          |
| ROI          | Focus patch after initial lock                    |
| FSM          | Finite State Machine governing fishing loop       |
| EMA          | Exponential moving average used in bite detection |
| RSS          | Resident Set Size (process working set)           |
