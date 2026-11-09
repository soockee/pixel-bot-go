---
title: "Fishing vision experiment results"
summary: "Baseline and experimental detector results from Fake Game and two recorded sessions."
order: 5
---

The RMS baseline matched 13 of 18 recorded bite labels. Spray history matched 6, waveform matching matched 3, and the combined variant matched 1. The experimental variants remain selectable, with the baseline as the default.

## Fake Game

The test uses the Fake Game renderer, a 33 ms frame interval and 10 ms audio packets. Four positive trials contain the same scripted spray with each of the three bundled sounds or with muted audio. Four negative trials contain local spray and unrelated audio, a whole-crop flash with a neighbouring bite sound, one frame of spray with a neighbouring bite sound, or a bite sound alone. The waveform matcher sees scaled copies of the same assets from which its templates were built.

| Bite detector | Detected bites | False triggers | Decision delay with bite audio | Muted delay |
| --- | ---: | ---: | ---: | ---: |
| `splash_sound` | 4/4 | 3/4 | 340–370 ms | 0 ms |
| `splash_history` | 4/4 | 1/4 | 340–370 ms | 33 ms |
| `splash_template_sound` | 4/4 | 2/4 | 480 ms | 0 ms |
| `splash_history_sound` | 4/4 | 0/4 | 480 ms | 33 ms |

History rejected the whole-crop flash and single-frame spray. Template matching rejected the unrelated sound. Matching the sample took 110–140 ms longer than the RMS onset in these trials.

| Unmarked Fake Game locator | Correct first locks | Clean first lock | First lock with another bobber present |
| --- | ---: | ---: | --- |
| `color` | 1/2 | 132 ms | Wrong bobber immediately |
| `ncc` | 1/2 | 132 ms | Wrong bobber immediately |
| `cast_track` | 2/2 | 198 ms | Correct bobber at 198 ms |

The new bobber appears at 132 ms in both locator trials. The cast tracker needs a second observation and reacquired on the second visible frame after a one-frame occlusion in this test. The live engine uses the locator for acquisition and then watches the locked crop.

## Recorded sessions

- `20261108-012132` has 2,862 frames at a median 100 ms interval. Replay used the ten existing bite labels and matched all ten cast timestamps to the input trace. Most frames were cropped around the addon marker.
- `20261108-035820` has 648 full frames at a median 295 ms interval. The `mouse:x1` cast binding and right-click reels produced eight automatic labels. The original recordings and labels were preserved.

Both sessions label every evaluated cast as having a bite. A reported false trigger is too early for the estimated bite time. The recordings provide no labelled negative casts for estimating an operational false-trigger rate. Bite timestamps come from an assumed reaction time, so the table uses matches within the evaluator's timing window. Bobber boxes come from click positions. The locator results measure agreement with those boxes in footage that can contain addon markers.

| Bite detector | Existing labels, matches / early triggers | Automatic labels, matches / early triggers |
| --- | ---: | ---: |
| `splash_sound` | 8/10 / 0 | 5/8 / 0 |
| `splash_history` | 6/10 / 0 | 0/8 / 0 |
| `splash_template_sound` | 2/10 / 2 | 1/8 / 0 |
| `splash_history_sound` | 1/10 / 0 | 0/8 / 0 |

The history prototype requires two observations within 150 ms. The second recording's median frame gap of 295 ms exceeds that window, so its zero matches chiefly expose a capture-rate mismatch. The first recording can exercise the two-frame gate and still loses two label matches relative to baseline. Waveform matching performed poorly in both recordings. The synthetic results establish that the matcher can recognize a clean, scaled copy of an asset.

| Locator | Existing labels, correct / total locks | Automatic labels, correct / total locks |
| --- | ---: | ---: |
| `marker` | 6/10 | 3/8 |
| `color` | 0/9 | 0/5 |
| `ncc` | 0/0 | 0/8 |
| `cast_track` | 0/9 | 0/2 |

In the second session, `color` matched 2 of 10 empty frames and NCC matched all 10. The cast tracker is idle outside a cast. It reduced the number of locks but produced zero correct first locks against the available boxes. Even the marker agreed with only 9 of 18 click-derived boxes. Assessing ownership needs continuous unmarked footage and independently verified bobber positions.

## Reproduce

```powershell
go test ./cmd/detecteval -run TestFakeGameVision -count=1 -v
go run ./cmd/detecteval -json -locator marker,color,ncc,cast_track -bite all <labelled-recording>
```

The existing labels were replayed from a copy with their matching `cast_ms` fields added. The second session was labelled with `detecteval -label` using the default mouse bindings. See the [recording and test plan](../how-to/test-fishing-vision.md) for the native-rate, independently labelled holdout that these recordings cannot provide.
