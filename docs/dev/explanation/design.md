---
title: "Design"
summary: "Why the state machine, the event loop and the input safety rules look the way they do."
order: 2
---

## FSM Lifecycle

```
waiting_focus → searching → monitoring → reeling → cooldown → casting → searching → …
```
A search deadline forces progress. Monitoring is lighter than searching because coordinates are known and only a small ROI is watched.

## Concurrency Snapshot

Core goroutines/tasks typically active during operation:
* Engine (one loop: grab a frame, run detection, feed the FSM)
* Scale match workers (spawn per scale; short-lived)
* FSM event loop (state transitions, deadline, input)
* Focus watcher (active from Start until the run halts)
* Optional debug loggers (goroutines & memory)

Use profiling endpoints to confirm counts remain stable; runaway worker spawning indicates misconfigured scale range.

## Design Rationale

Separate searching vs monitoring → lower cost post‑lock. Timers decouple cadence from UI ticks. Short explicit states (`casting`, `reeling`) improve observability. Single event channel serializes transitions avoiding races. The Engine runs without the UI, so a slow preview cannot delay a bite, and tests can replay the whole pipeline.

## Input Safety

The app sends commands to the ESP32 over USB serial. The board emits the corresponding Bluetooth keyboard and mouse reports. Fishing actions run on the FSM goroutine. Leaving a state cancels its deadline, and the app checks that the selected game window has focus before sending an action. Scheduled login and logout recheck the screen and focus after opening the board's serial port.

`action.Guard` checks pointer targets against the game window's bounds and detects when the pointer has moved more than 6 pixels from where Go Fish left it. Small changes from mouse sensitivity or the game are allowed. Larger moves stop the run and log the expected and actual positions. Runs also stop on request, after too many failed casts, or at the session limit.

## Extensibility Ideas

Adaptive thresholds, variable cooldown strategies, multi‑modal confirmation (audio splash) can be added as new event sources feeding the FSM.

`splash_sound` takes a splash to be white spray at the centre of the marker box, where the bobber sits: centre-disc pixels at least 12 grey levels lighter than a baseline that takes on the scene with a 0.5 s lag, and nearly colourless (at most 25 between the colour channels), covering at least 9 % of the disc and at least twice the 90th percentile of the share over the last 3 s. On open water that second bar stays under 9 %, but a fishing pool's animated foam keeps 10 to 16 % of the centre sprayed (up to 26 % without a bite in a recorded pool run, where a fixed 9 % gave false bites in 3 of 8 casts), so there the bar rises to 20 to 30 % while a bite still reaches 24 to 40 %. The lag lets lasting changes such as the landed bobber or a parked cursor blend in, while spray stands out within a frame or two; the first 3 s after the lock, while the bobber lands, are ignored. A pet, a player or the fishing line passing through the box is dark and a spell effect is coloured, so none of them counts, and a measure per frame does not depend on the frame rate. On a recorded bot run, labelled from the game's own outcomes, a bite's spray covered 10 to 25 % of the disc and nothing else more than about 6 %; the passing pet and the ripples behind its two "No fish are hooked" reels stayed below that. The earlier cues, any change at the centre against its recent level and a bright splash 35 levels over the baseline, missed soft splashes whenever the fishing line swinging through the box had raised the change level, and took the pet and the ripples for splashes. A splash that decides on its own (the fallbacks below) also needs half of its spray at the centre, so spray of another bobber in the box does not count.

The `splash_sound` bite detector, the only one, adds the game's sound. In recordings the bite sound starts 0.2 to 0.45 s after the splash at the player's bobber. Other players' bobbers make the same sound but splash elsewhere, and spell effects splash without it, so the detector needs both: a sound starting within 0.8 s after a splash in the centre of the box. A splash shows for several frames and the sound often starts while it still shows, and the sound arrives a little after the frames of the same moment, so the window runs from the splash's first frame to 0.8 s after its last. Both fallbacks remember the run, not the cast. When the game has made no sound at all since the run started (no 50 ms level of 1 or more), for example with its sound switched off, the splash alone reports the bite once the box has been watched for 3 s. When the game makes sound but no bite sound, for example with sound effects off and music on, three casts in a row whose splash brought no bite mark the game as one without the bite sound, and from then on the splash alone reports the bite until the run stops; once a bite was confirmed by its sound this never happens. A game with sound effects on is silent between them (recorded hand play: a 50 ms level of about 0.5 between sounds, and a bite sound of 15 to 40 at that game volume 0.3 to 1.1 s before every reel), so the earlier per-cast rules took a quiet cast for a silent game, and one unanswered splash for a game without the bite sound: on four labelled hand-play recordings (52 casts) that gave false bites in 35 % of casts, the run-wide rules 13.5 %, with bites caught up from 54 % to 65 %. A sound starts when its 50 ms level is above 15 and three times the median of the 2 s before. The bite sound swells over 0.2 to 0.5 s, so in a noisy game it crosses that line up to 0.3 s late; the onset is dated back over the steps just before that were already 1.5 times the background, which leaves at most 175 ms on the recorded bite sounds (`assets/sounds/`). A sound then lasts until it falls back to 1.5 times the background, so the swells inside one bite sound start no new sound: without that, another player's bite sound that began just before the splash swelled again inside the window and counted. A bite sound can still begin while another sound plays, such as wind that rose before the splash and never fell back: in a recorded cast the only onset then lay 0.74 s before the splash and the bite was missed. So once a sound has played 0.7 s, a level of twice its loudest step before the last 300 ms starts a new sound, dated at the quietest of those 300 ms. The recorded bite sounds swell for up to 0.5 s and later stay within 1.5 times their peak, so each is still one sound; on the labelled recordings (95 casts) bites caught went from 73.3 % to 74.4 % with false bites unchanged at 3.2 %. The engine taps the selected window's process through Windows process loopback, which leaves the game audible and records no other program. The game mutes itself in the background unless its sound-in-background option is on, but the bot only fishes while the game is focused.

The `color` locator finds the bobber without the addon. It takes the water's colour as the CIELAB mean of the pixels near the median of a band above the character, rates every pixel as a red feather (R-(G+B)), a blue feather (B-(R+G)) or the tan body (bright, hue near 35 degrees, saturation near 0.35), and keeps the strongest of each. Two pixels of different classes within 25 px that both stand out from the water by 15 CIELAB units are the bobber; it tries the top 5 of each class first and doubles up to 160. It is fooled by anything with two such colours close together: in the labelled recording it locked onto the Glimmer addon's gold icon above the bobber in every cast, and at night the bobber shows few coloured pixels. Use it only where the addon cannot be installed.

## Limitations

Lighting shifts, motion blur, busy backgrounds, rotation/perspective changes, extreme scale ranges degrade accuracy or performance. The approach is 2D template matching + light motion heuristics (not full CV/ML).
