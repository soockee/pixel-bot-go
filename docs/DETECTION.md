# Object Detection Algorithm

This document explains the multi-scale template matching algorithm used to locate the target object (e.g. fishing bobber) in captured screen frames. The GUI and screen capture flow are simple wrappers around this core routine.

## Overview

The detector performs masked, normalized cross-correlation (NCC) between a template image and the current screen frame across a range of scale factors. It returns the best match (highest correlation score) subject to a configurable threshold. The algorithm is robust to moderate size changes, transparency in the template, and optional color-channel differences.

Core components:

1. Single-scale NCC (`MatchTemplateNCC`)
2. Parallel multi-scale orchestration (`MultiScaleMatchParallel`)
3. Adaptive scale generation (`MultiScaleOptions` with `MinScale`, `MaxScale`, `ScaleStep`)
4. Optional RGB correlation (`UseRGB`) for increased precision when color patterns matter.

## Data Structures

### `NCCOptions`
- `Threshold`: Minimum score to consider a match valid.
- `Stride`: Coarse scan step in pixels (trade-off between speed and accuracy).
- `Refine`: If true and `Stride > 1`, performs a local refinement pass around the best coarse candidate.
- `ReturnBestEven`: If true, returns best coordinates even if score < threshold (caller uses `Found` flag).
- `DebugTiming`: Collects timing info (duration only).
- `UseRGB`: Use R,G,B channels separately then average channel correlations; improves robustness vs. grayscale.

### `MultiScaleOptions`
- `Scales`: Explicit list of scale factors; if empty, adaptive generation is used.
- `MinScale`, `MaxScale`, `ScaleStep`: Define continuous range of scales if `Scales` not provided.
- `NCC`: Options passed down to NCC for each scale.
- `StopOnScore`: Early termination threshold (e.g. >= 0.95).

### `MultiScaleResult` / `NCCResult`
Contain coordinates (`X`,`Y`), best `Score`, `Scale` (for multi-scale), `Found` flag, and optional `Dur`.

## Single-Scale NCC Details

For a given frame `F` and template `T` (possibly resized), NCC computes:

\[ \text{score} = \frac{\sum (F_i T_i) - n \bar{F}\bar{T}}{n \sigma_F \sigma_T} \]

Where:
- `i` indexes masked template pixels (transparent template pixels are skipped).
- `n` is number of unmasked pixels.
- `\bar{F}` and `\bar{T}` are means over those pixels.
- `\sigma_F`, `\sigma_T` are standard deviations.

The template and frame are converted either to a single grayscale luminance channel or three separate channels. Transparent pixels (alpha == 0) provide a natural mask to exclude irrelevant regions.

### Performance Considerations
- A stride (`Stride`) reduces the number of candidate positions during the coarse pass.
- A refinement pass re-scans fully around the best coarse location with stride 1 (or smaller effective region) to improve output accuracy.
- Early constant-template shortcut: if variance of template is near zero, matching degrades to equality check.

## Multi-Scale Matching

Size variability is addressed by scaling the template across multiple factors. For each scale factor:

1. Compute scaled width/height.
2. Resample template using Catmull-Rom interpolation.
3. Run NCC and record the best score.
4. If `StopOnScore` reached, trigger atomic early stop.

Concurrency model:
- Each scale factor executes in a goroutine.
- A bounded semaphore (`runtime.NumCPU()`) limits simultaneous workers to avoid overwhelming the CPU.
- A channel collects results; early-stop sets an atomic flag causing remaining goroutines to skip work.

Adaptive scales are generated when the caller does not provide an explicit list:
```
for s := MinScale; s <= MaxScale; s += ScaleStep { append(scales, s) }
```
Safety cap prevents pathological ranges (e.g. thousands of scales).

## Color (RGB) vs Grayscale Matching

In some game scenes, grayscale collapses distinguishing information (e.g. similarly bright but differently colored UI elements). Setting `UseRGB` performs NCC independently per channel and averages the three resulting correlations. This mitigates false positives where only one channel correlates by chance.

Trade-offs:
- Slightly higher memory and CPU cost (three arrays instead of one).
- Better discrimination for colorful targets against noisy backgrounds.

## Typical Configuration Example

```go
opts := capture.MultiScaleOptions{
    MinScale:   0.60,
    MaxScale:   1.40,
    ScaleStep:  0.05,
    NCC: capture.NCCOptions{
        Threshold:      0.80,
        Stride:         4,
        Refine:         true,
        ReturnBestEven: true,
        UseRGB:         true,
    },
    StopOnScore: 0.95,
}
result := capture.MultiScaleMatch(frame, template, opts)
if result.Found {
    // Use result.X, result.Y
}
```

## Edge Cases & Handling
| Case                                   | Handling                                                                  |
| -------------------------------------- | ------------------------------------------------------------------------- |
| Template fully transparent             | Early exit (no pixels).                                                   |
| Constant template                      | Equality shortcut; score set to 1 on match.                               |
| Scale produces very small (w<2 or h<2) | Ignored.                                                                  |
| Threshold unreachable                  | `Found` false; optionally returns best coordinates (if `ReturnBestEven`). |
| High score encountered                 | Early-stop prevents unnecessary remaining scale work.                     |

## Potential Future Enhancements
- Integrate integral images for O(1) mean/variance updates per window.
- Multi-stage pyramid: coarse detection at lower resolutions then refine at full scale.
- Sub-pixel peak fitting (e.g. quadratic fit around best NCC response) for finer cursor placement.
- Top-N candidate list with secondary verification (color histogram or edge gradient matching).
- Dynamic stride: start large, progressively tighten around promising areas.

## Interaction With GUI & Capture

Screen capture supplies frames; the GUI periodically pulls latest frame and invokes the detector. On a positive match, the application moves the mouse cursor to the detected location. The GUI also shows a small preview and textual debug status.

---
*This algorithm documentation focuses solely on the detection engine; peripheral components (Tk GUI and capture loop) are lightweight wrappers.*
