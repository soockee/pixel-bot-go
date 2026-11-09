---
title: "Detection datasets"
summary: "The recording format detecteval reads, and its labels."
order: 7
---

`-record` writes `frames/<elapsed ms>.png`, every one with the window's size and coordinates: the whole window once every 10 seconds, and in between, while the marker shows, only a square around the marker box (at least 256 px, 32 px beyond the marker) with the rest black, about 10 a second; frames without a marker in between are skipped. Black compresses to almost nothing, so a bobber frame takes about 0.3 MB instead of several MB, and readers need no crop offsets. With `-window` it also writes `input.jsonl` and `audio.wav`. Frame names, trace times and audio count from the same start: sample n of `audio.wav` (16-bit PCM, stereo, 44.1 kHz) played n/44100 s after it, and silence fills the time the game played nothing. The sound is a copy of the game process and its child processes only (Windows process loopback, Windows 10 2004 or later); the game stays audible and no other program's sound is recorded. If the sound cannot be captured, the status says so and the recording continues without it. and `session.json` holds the offset from window positions to frame pixels. Coordinates in `labels.json` are pixels in the frames. Write it by hand or generate it with `detecteval -label`.

Use `-record-full` for vision experiments. It targets 30 fps and saves the full client area on every successful capture, including before the cast and when the marker is hidden. Use the PNG timestamps to measure the actual frame cadence. [Experiment protocol](../how-to/test-fishing-vision.md)

```json
{
  "casts": [
    {"start_ms": 1200, "end_ms": 15800, "bobber": {"x": 212, "y": 148, "w": 24, "h": 18}, "bite_ms": 12400},
    {"start_ms": 21000, "end_ms": 37000, "bobber": {"x": 240, "y": 160, "w": 24, "h": 18}}
  ],
  "empty": [{"start_ms": 16000, "end_ms": 20000}]
}
```

A cast covers the time the bobber is on screen. Leave out `bite_ms` if no bite came. Add `cast_ms` when the recorded input gives the cast time. Locator replay starts there, before landing. For older labels, `cast_track` starts two seconds before `start_ms` to obtain a reference frame. `empty` lists spans with no bobber. Cast-conditioned locators are idle between casts.

`go run ./cmd/detecteval [-config file] [-locator names] [-bite names] [-json] dir...` compares techniques across all the datasets given and prints one column per technique. `-locator` and `-bite` take comma-separated names, and both default to `all`:

| Metric | Meaning |
| ------ | ------- |
| Bobber found per cast | Casts where any frame gave a match |
| False locks on empty frames | Matches in `empty` spans, per frame |
| Click point inside the bobber | First matches of a cast inside the labelled rectangle, per lock |
| Bites caught | First trigger from 200 ms before `bite_ms` to 1500 ms after it |
| False bites per cast | First trigger earlier than that, or in a cast with no bite |
| Bite latency p95 | Trigger time after `bite_ms`, rounded up to 0 for early triggers |

A detector that listens, such as `splash_sound`, also gets the session's `audio.wav` in play order, and datasets without sound are skipped for it. Bite metrics watch the marker box from the frame it locks on, landing included, as the app does, and otherwise a square around the labelled bobber from the cast's start. Every bite detector sees the same areas, and the choice of locator does not affect them. `n/a` means nothing was measured. All values except search time repeat exactly. Recordings run at about 10 fps and the live app at 30 fps, so bite numbers are an approximation. `bite_ms` from `-label` is the reel minus a fixed reaction time, while real reactions vary from 0.8 to 1.7 s, so a trigger on the real splash can count as a false bite.

`detecteval -label <dir> [-cast mouse:x1] [-reel mouse:right] [-reaction 1200] [-box 40]` pairs each cast with the next reel click. Bindings are `mouse:<button>` for a click or `key:<name>`, including chords such as `key:Shift+B`.

| Label | Value |
| ----- | ----- |
| Cast span | From 2 s after the cast, once the bobber has landed, to the reel click. `cast_ms` records the cast input time. |
| `bobber` | A `-box` px square around the reel click |
| `bite_ms` | The reel click minus `-reaction` |
| `empty` | From 1 s after a reel to the next cast, if at least 1 s long |

A cast with no reel, a reel within the first 2 s, and a click outside the frame are skipped and listed. Every labelled cast has a bite, so false bites can only count against early triggers.

`detecteval -label-bot <dir> [-box 40]` labels a recording of a bot run (Record while the bot fishes) from its `bot.jsonl`, where Bobber Marker 1.5.1 or later reports how each reel ended. A reel that caught a fish or let it get away followed a real bite, labelled at the splash: the detection less the splash-to-sound gap the detector logged. A reel with "No fish are hooked" followed a false bite, and its cast is labelled without one. Each cast spans from the lock to the reel. Casts that ran out, deliberate misses and reels without an outcome are listed as skipped: whether a bite came there is for a person to check in the frames and add by hand. `bot.jsonl` gives the bobber in screen coordinates, which match the frames when the game window sits at the screen's origin; the bite metrics watch the locked marker box anyway.

`detecteval -export <new dir> dir...` saves each cast's watched area, from the cast until the reel, as `<dataset>/cast-NNN/<ms>.png`. `index.jsonl` has one row per crop with `file`, `dataset`, `cast`, `ms`, `box`, `reel_ms` and the estimated `bite_ms`.
