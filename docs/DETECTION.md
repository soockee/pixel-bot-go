# Detection Guide (Beginner-Friendly)

This guide explains how Pixel Bot spots a target image (for example a small game object) on your screen. It is written for CS students at a beginner to intermediate level. You do NOT need prior experience in machine learning or advanced computer vision.

## 1. The Problem
Given a larger screenshot (the frame) and a smaller picture (the template), we want to know: "Where does this smaller picture appear in the larger one?" If we find a good match we move the mouse there.

## 2. Core Idea: Compare Small Patch Against the Screen
We slide (imagine moving) the template over the screen and at each possible position we measure how similar they look. The best similarity score wins.

## 3. Similarity via Normalized Cross-Correlation (NCC)
NCC gives a score from -1 to +1 where:
- +1 means "identical pattern" (perfect match)
- 0 means "no clear relationship"
- -1 means "inverted pattern" (not relevant here)

Simplified formula (details at the link below):

```math
	ext{score} = \frac{\sum_{i=1}^{n} F_i T_i - n\,\bar F\,\bar T}{n\,\sigma_F\,\sigma_T}
```

Where:
- $F_i$ is the $i$-th frame pixel under the template window (after masking)
- $T_i$ is the corresponding template pixel
- $n$ is the number of (non-transparent) template pixels used
- $\bar F, \bar T$ are the means of $F_i$ and $T_i$
- $\sigma_F, \sigma_T$ are their standard deviations

We only use pixels from the non-transparent parts of the template (so see-through areas are ignored). Normalization (the denominator) makes the score independent of lighting differences.

Further reading (optional):
https://xcdskd.readthedocs.io/en/latest/cross_correlation/cross_correlation_coefficient.html

## 4. Why We Also Change Scale
Sometimes the object on screen appears slightly bigger or smaller than our stored template (distance, resolution changes, etc.). To handle that we try several scale factors: we resize the template, run NCC, keep the best score.

## 5. RGB vs Grayscale
- Grayscale: Convert colors to brightness and compare one channel (faster).
- RGB: Compare Red, Green, Blue separately and average their scores (slower, but better when color distinguishes objects).

If colors matter (often in games), using RGB reduces false positives.

## 6. Making It Fast
Searching every pixel at every scale can be slow. We use two tricks:
1. Stride: skip some positions (e.g. move 4 pixels at a time) for a quick coarse scan.
2. Refinement: after finding the best coarse spot, re-check a small region precisely (stride 1) to fine-tune coordinates.

We also stop early if a score is "good enough" (StopOnScore) to save time.

## 7. Putting It Together (Pseudo-code)
```pseudo
best = none
for scale in generated_scales:
    scaledTemplate = resize(template, scale)
    result = NCC_Search(frame, scaledTemplate, stride)
    if result.score > best.score:
        best = result with scale
    if best.score >= StopOnScore:
        break (early stop)
if stride > 1 and Refine:
    best = refine_search(frame, scaledTemplate, around best.position)
return best
```

`NCC_Search` loops over candidate positions and computes the NCC score for each; `refine_search` only looks near the best coarse position.

## 8. Configuration (Plain Terms)
| Setting             | What It Means                                     | Typical Effect                                         |
| ------------------- | ------------------------------------------------- | ------------------------------------------------------ |
| MinScale / MaxScale | Smallest and largest size to try for the template | Wider range handles more size variation but costs time |
| ScaleStep           | How big each jump is when scaling                 | Smaller step = more scales = slower but thorough       |
| Threshold           | Minimum score to call it a "found" match          | Higher = fewer false positives                         |
| Stride              | Pixel step when scanning                          | Larger stride = faster, might miss best exact spot     |
| Refine              | Re-check around best spot with stride 1           | Improves coordinate accuracy                           |
| UseRGB              | Use color channels                                | Better precision; slightly slower                      |
| StopOnScore         | Early stop score                                  | Saves time once confidence is high                     |
| ReturnBestEven      | Provide best coordinates even if below threshold  | Useful for debugging                                   |

## 9. Edge Cases
- Fully transparent template: nothing to match → skip.
- Template with no variation (e.g. all one color): correlation degenerates → special handling (treated like equality check).
- Too small after scaling (<2 pixels wide or high): skip (not meaningful).
- No score above threshold: report "not found" (or still give best if `ReturnBestEven`).

## 10. Performance Notes
- More scales or smaller strides increase CPU time.
- Early stopping plus refinement keeps interaction responsive.
- Parallel workers process different scales concurrently (bounded by CPU cores).

## 11. Limitations
- Lighting or heavy motion blur can reduce score reliability.
- Very busy backgrounds may produce false positives (raise Threshold or use RGB).
- Large perspective changes (object tilted or rotated) are not handled.

## 12. Possible Future Improvements (Optional Reading)
- Integral images to speed mean/variance calculations.
- Multi-resolution pyramid: search coarse, then refine at full resolution.
- Sub-pixel estimation to get smoother cursor placement.
- Multiple top candidates for secondary checks (color histogram, edges).

## 13. Relationship to the GUI
The GUI just feeds new frames into this detection pipeline and displays status. When a match passes the threshold the cursor moves to the match coordinates.

---
If you understand the sections above you already grasp the core of template matching with NCC at an approachable level. Dive into the source if you want more detail.
