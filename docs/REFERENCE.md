# Reference

Central factual lookups: algorithms, config parameters, state/event tables, profiling endpoints.

---
## NCC Formula
The normalized cross-correlation score used in template matching:
$$
	ext{score} = \frac{\sum_i F_i T_i - n\,\mu_F\,\mu_T}{n\,\sigma_F\,\sigma_T}
$$
Where:
* $F_i$ – frame pixel value at position $i$ (after any grayscale or channel selection)
* $T_i$ – template pixel value at position $i$ (transparent pixels omitted)
* $n$ – number of contributing pixels
* $\mu_F, \mu_T$ – means of frame patch and template
* $\sigma_F, \sigma_T$ – standard deviations

Normalization removes brightness bias; higher score means better similarity (range roughly -1..+1; negative/inverted ignored here).

---
## Detection Goal
Locate template (small) within frame (large) returning (x,y,score,scale,found?). Score threshold gates action.

## Performance Techniques
1. Integral images for O(1) sums/variance.
2. Stride coarse pass + optional refine.
3. Early stop on `StopOnScore`.
4. Parallel scale workers; race ends early on high score.
5. Template transparency ignored; brightness normalized.

## Algorithm Flow
```text
generate scale list
for each scale in parallel:
    build / reuse scaled template
    run NCC with (stride, threshold)
    send best score for that scale
collect results
pick overall best
if refine enabled and stride > 1:
    locally re-scan around best with stride=1
return (x,y,score,scale,found?)
```

## Config Parameters
| Setting                     | Meaning                                     | Tradeoff                               |
| --------------------------- | ------------------------------------------- | -------------------------------------- |
| Threshold                   | Minimum NCC score considered a hit          | ↑ fewer false positives, ↓ sensitivity |
| Stride                      | Pixel step while scanning                   | ↑ faster, ↓ coarse precision           |
| Refine                      | Precise second pass around best coarse spot | Slight cost, better accuracy           |
| MinScale/MaxScale/ScaleStep | Scale search range                          | Wide + tiny step = heavier workload    |
| StopOnScore                 | Early exit threshold                        | Saves time if early strong match       |
| ReturnBestEven              | Return coords even below threshold          | Aids tuning & diagnostics              |

## Capabilities
* Watch a screen region for the bobber template.
* Multi-scale template matching with stride + refine pass.
* Automated fishing loop (cast → search → monitor → reel → cooldown).
* Bite detection via grayscale ROI motion heuristics.
* Dark mode UI and adjustable selection area.
* JSON config persistence and structured event logging.

## Generated Files
| File                    | Purpose                                  | Notes                                                   |
| ----------------------- | ---------------------------------------- | ------------------------------------------------------- |
| `pixle_bot_config.json` | Persist user settings between runs       | Safe to edit while app closed; delete to reset defaults |
| `pixel_bot_logs.json`   | Structured log of events & state changes | Can be deleted; recreated automatically                 |

## FAQ
| Question                               | Answer                                                                    |
| -------------------------------------- | ------------------------------------------------------------------------- |
| Do I need to tweak settings first run? | No, defaults usually work.                                                |
| Why run as Administrator?              | Required for reliable mouse movement and key injection on Windows.        |
| It misses bites or reels late          | Lower `Threshold` slightly or reduce `Stride`; ensure refine enabled.     |
| False positives on water ripples       | Raise `Threshold` or narrow selection area.                               |
| Slow over time                         | Reapply settings or reduce scale range; profile for hotspots.             |
| How to reset everything?               | Close app, delete `pixle_bot_config.json` (and optionally logs), restart. |

## State Machine: States
| State         | Meaning                 | Exit Condition                              |
| ------------- | ----------------------- | ------------------------------------------- |
| halt          | Idle                    | Enable capture → waiting_focus              |
| waiting_focus | Await game window focus | Focus → searching                           |
| searching     | Scan template scales    | Found → monitoring; timeout/force → casting |
| monitoring    | Watch bobber motion     | Bite → reeling; lost → casting              |
| reeling       | Perform reel action     | Done → cooldown                             |
| cooldown      | Delay before next cast  | Timer → casting                             |
| casting       | Send cast key           | Immediate internal → searching              |

## State Machine: Events
| Event                    | Source               | Effect                          |
| ------------------------ | -------------------- | ------------------------------- |
| EventAwaitFocus          | User enables capture | halt → waiting_focus            |
| EventFocusAcquired       | Focus watcher        | waiting_focus → searching       |
| EventTargetAcquired / At | Detection            | searching → monitoring          |
| EventTargetLost          | Motion heuristic     | monitoring → casting            |
| EventFishBite            | Bite detector        | monitoring → reeling → cooldown |
| ForceCast                | Timer / user         | any (except casting) → casting  |
| EventHalt                | User stops           | any → halt                      |

## Profiling Endpoints (pprof)
| Endpoint                        | Purpose            |
| ------------------------------- | ------------------ |
| /debug/pprof/                   | Index page         |
| /debug/pprof/profile?seconds=30 | CPU profile window |
| /debug/pprof/heap               | Heap allocations   |
| /debug/pprof/goroutine          | Goroutine stacks   |
| /debug/pprof/block              | Blocking profile   |
| /debug/pprof/mutex              | Mutex contention   |

## Common Fix Patterns
Reuse buffers; tune scale range; batch work instead of per-op goroutines; introduce backpressure via bounded channels.

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

---
## Source Attribution
Content originally distributed across standalone detection, state machine, and debug guides now consolidated here. Former README details (capabilities, generated files, FAQ) also merged.
