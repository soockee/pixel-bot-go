---
title: "Test fishing vision techniques"
summary: "Capture comparable clips and test visual history, sound matching and cast tracking separately."
order: 6
---

## Record the two datasets

Build the feature branch and install its bundled Bobber Marker through Settings → Addon. Reload the game UI. Record short manual sessions with the game foregrounded and the window fixed. Use a fresh directory for each session and stop with Ctrl+C.

```powershell
.\pixel-bot-go.exe -record datasets/vision-marked-01 -window "<game window>" -record-full -record-keys
.\pixel-bot-go.exe -record datasets/vision-unmarked-01 -window "<game window>" -record-full -record-keys
```

1. For the **marked** session, use `/bobbermarker pixels on`. Record open water, foamy pools, nearby players, and at least one session with bite audio muted. Use the marker to establish the same bobber region for every bite detector.
2. Before the **unmarked** session, use `/bobbermarker pixels off` and verify the brackets and the top-left addon blocks are gone. This mode keeps the addon's game settings and binds active. For a separate addon-free holdout, disable the addon and reload, then arrange a manual cast binding. Capture from before each cast through landing, including already-present bobbers and overlapping casts. Restore visible pixels before using the normal bot again.
3. Check actual frame gaps from the filenames and keep sessions with significant gaps separate. Full-window PNGs consume substantially more space and may reduce the achieved capture rate. Use `input.jsonl` to find each cast keypress. For each cast, annotate the true bobber box, the last clearly pre-bite frame and first clearly visible bite frame, or a verified no-bite outcome. Include casts that time out. Enter the first visible frame in `labels.json` for existing tools and retain the interval when analysing latency. Keep entire sessions together when splitting tuning and holdout data.

## Plug in and compare the three techniques

| Experiment | Plugin and comparison | Pass condition |
| --- | --- | --- |
| **Spray history** | Register `splash_history` as a `SoundBiteDetector` so it can retain the existing audio confirmation. Change only the visual features. Replay it alongside `splash_sound` on identical marked crops. Include foam, landing ripples, neighbour splashes and muted audio. | Fewer false triggers at matched recall, or higher recall at the baseline false-trigger rate. Count all frames needed to make the decision in the latency. |
| **Bite sound matching** | Register `splash_template_sound` as a `SoundBiteDetector`. Reuse the baseline visual gate and compare short bundled-sample matches with the existing RMS onset. Include unrelated loud effects and genuine neighbouring bites that fall inside the timing window. Then test both changes together as a separate variant. | Fewer false confirmations without a meaningful p95 decision delay increase. Measure sound onset and the time enough samples were available to act separately. |
| **Cast-conditioned tracking** | Register `cast_track` as a `Locator`. When implementing it, add an optional per-cast hook called from the live search on actual cast dispatch and from replay using the recorded cast timestamp. Keep existing single-frame locators on their current interface. Use annotated unmarked footage and compare first locks against `color` and `ncc`. Continue replay after the lock to evaluate tracking and reacquisition, since `detecteval` currently stops at the first match. | More correct first locks at a matched wrong-bobber rate. Also report time from cast to first correct lock, drift and recovery after occlusion. |

`domain/detect/registry.go` is the registration point used by the app and `detecteval`. The names above are planned registrations. Run the baseline now, then add each candidate to the same replay command as it becomes available.

```powershell
go run ./cmd/detecteval -locator marker -bite splash_sound datasets/vision-marked-01
go run ./cmd/detecteval -locator color,ncc datasets/vision-unmarked-01
```

Read the locator results for the unmarked command. Run simulator and deterministic replay checks first, then use the held-out Windows captures. For bites, report recall, false triggers per cast and p50/p95 decision delay, broken out by water and audio mode. Use actual timestamps and no future frames. The existing evaluator's 1.5-second success window is a coarse check. For latency, compare trigger timestamps to the independent annotation intervals. Record CPU time and missed capture ticks alongside accuracy. After the separate comparisons, replay the selected locator and detector through the fishing state machine in dry run.

[Research and sources](../explanation/fishing-vision-research.md) · [Dataset format](../reference/datasets.md)
