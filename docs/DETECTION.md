# Detection Guide (Beginner-Friendly)

How Pixel Bot finds a small target (like a game object) inside a screenshot. Written for early CS grads—no advanced CV background required.

---
## 1. Goal (Plain)
You have:
- A live frame (big image).
- A template (small image you want to locate).

Question: Where (x,y) inside the frame does the template best fit? If score passes a threshold we treat it as "found" and move the cursor there.

---
## 2. Sliding Match Mental Model
Imagine laying the template on every possible position on the frame (like a stencil) and asking "How similar is this patch?" Keep the highest score.

Brute force? Yes—but we add math tricks and shortcuts so it stays fast.

---
## 3. Normalized Cross‑Correlation (NCC) in One Paragraph
NCC outputs a number between -1 and +1:
- Near +1 → pixels pattern matches strongly.
- Around 0 → unrelated.
- Negative → inverted (we can ignore for this use case).

We ignore fully transparent template pixels. We adjust for brightness differences by normalizing (subtract mean, divide by standard deviation). That way a dimmer or brighter frame area can still match.

Formula (reference only):
$$
	ext{score} = \frac{\sum_i F_i T_i - n\,\mu_F\,\mu_T}{n\,\sigma_F\,\sigma_T}
$$
Where `n` is the pixel count used. You do NOT need to derive it—just know higher means a better match.

---
## 4. Multi‑Scale Search (Why Resize?)
The object can appear slightly bigger or smaller (resolution changes, distance, UI scaling). We therefore test several scale factors of the template. Each scaled version is matched; we keep the overall best result.

Scales are generated from config: `MinScale .. MaxScale` in steps of `ScaleStep`. More scales = more work.

---
## 5. Speed Tricks
1. Integral Images: Precompute fast sum/variance lookups for any rectangular patch (O(1) per window). This speeds the math for NCC.
2. Stride: Skip pixels (e.g. check every 4th position). Coarse first pass.
3. Refinement: After coarse pass, zoom back in near the best spot with stride 1 for precise coordinates.
4. Early Stop: If a score reaches `StopOnScore` we bail out early.
5. Parallel Scales: Each scale factor can run in its own goroutine; the first high scorer may end the race early.

Result: Even multiple scales stay responsive.

---
## 6. Simplified Flow
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

---
## 7. Key Config Terms
| Setting                       | Plain Meaning                                  | Tradeoff                                          |
| ----------------------------- | ---------------------------------------------- | ------------------------------------------------- |
| `Threshold`                   | Minimum NCC score to count as found            | Higher = fewer false positives, maybe fewer finds |
| `Stride`                      | Step size when sliding                         | Bigger = faster, less precise coarse pass         |
| `Refine`                      | Do precise second pass around best coarse spot | Small added cost, better accuracy                 |
| `MinScale/MaxScale/ScaleStep` | Range of scales tested                         | Wide range + tiny step = many goroutines/time     |
| `StopOnScore`                 | Early exit when score >= value                 | Saves time if good match appears early            |
| `ReturnBestEven`              | Still return coords even if below threshold    | Helpful for tuning                                |

---
## 8. Interaction With Fishing FSM
Once detection finds the target:
1. Coordinates are sent to the Fishing FSM (state machine).
2. FSM shifts from Searching → Monitoring; cursor may move to the match.
3. While Monitoring, small Region Of Interest (ROI) frames feed the `BiteDetector` which looks for pixel change patterns (spikes, baseline shifts) to decide if a fish bite occurred.
4. Bite event triggers Reeling actions (move + click) then cooldown.

So: Template match gets you initial coordinates; BiteDetector watches subtle pixel changes over time.

---
## 9. BiteDetector (High-Level)
It converts each ROI frame to a fast grayscale byte slice, tracks how much each frame differs from a rolling average (EMA) and a short statistics window, and looks for spikes or jumps with ratio thresholds. When enough consecutive "candidate" frames appear (or one big early spike) it fires. Simpler than ML; just statistical heuristics.

---
## 10. Metrics & Debugging Hooks
With `debug` enabled you can optionally enable goroutine/memory loggers (see `DEBUG.md`). They help you confirm:
- Parallel scales aren't leaking goroutines.
- NCC loop times are within expected bounds (use CPU profile).
- BiteDetector doesn't allocate excessively per frame (should reuse slices).

Timing data (`DebugTiming`) collects durations for scale matches so you can see if one scale dominates cost.

---
## 11. Limitations
- Lighting changes or heavy motion blur can lower scores.
- Busy backgrounds may require a higher `Threshold` to avoid false positives.
- Rotation, perspective warp, or large 3D depth changes are not handled.
- Extreme scale ranges (e.g. 0.2 → 3.0 tiny steps) will spawn many workers and slow results.

Tuning tips:
- Start with moderate stride (2–4). If misses increase, lower it or enable `Refine`.
- Narrow scale range if object size is fairly stable.

---
## 12. Glossary
- Template: The small image you want to find.
- Frame: The full captured screenshot.
- Scale Factor: Multiplier to resize template (1.2 = 20% larger).
- Stride: Pixel step size while scanning.
- NCC: Normalized cross‑correlation, similarity score method.
- ROI: Small patch of the frame watched for subtle changes (used after initial match).
- FSM: Finite State Machine ruling fishing states.

---
## 13. Quick Sanity Checklist
| Symptom                         | What to Adjust                        |
| ------------------------------- | ------------------------------------- |
| False positives                 | Raise `Threshold`, enable `Refine`    |
| Slow detection                  | Increase `Stride`, narrow scale range |
| Missed target at different size | Expand `MinScale..MaxScale` range     |
| Memory spike                    | Reduce number of scales; profile heap |

---
If you reached here you understand the core mechanics and how config affects speed vs accuracy. Explore the source (`domain/capture/*.go`, `domain/fishing/*.go`) for deeper details.
