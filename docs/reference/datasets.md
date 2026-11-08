---
title: "Detection datasets"
summary: "The recording format detecteval reads, and its labels."
order: 7
---

`-record` writes `frames/<elapsed ms>.png`, and `input.jsonl` and `audio.wav` with `-window`. Frame names, trace times and audio count from the same start: sample n of `audio.wav` (16-bit PCM, stereo, 44.1 kHz) played n/44100 s after it, and silence fills the time the game played nothing. The sound is a copy of the game process and its child processes only (Windows process loopback, Windows 10 2004 or later); the game stays audible and no other program's sound is recorded. If the sound cannot be captured, the status says so and the recording continues without it. and `session.json` holds the offset from window positions to frame pixels. Coordinates in `labels.json` are pixels in the frames. Write it by hand or generate it with `detecteval -label`.

```json
{
  "casts": [
    {"start_ms": 1200, "end_ms": 15800, "bobber": {"x": 212, "y": 148, "w": 24, "h": 18}, "bite_ms": 12400},
    {"start_ms": 21000, "end_ms": 37000, "bobber": {"x": 240, "y": 160, "w": 24, "h": 18}}
  ],
  "empty": [{"start_ms": 16000, "end_ms": 20000}]
}
```

A cast covers the time the bobber is on screen. Leave out `bite_ms` if no bite came. `empty` lists spans with no bobber.

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
| Cast span | From 2 s after the cast, once the bobber has landed, to the reel click |
| `bobber` | A `-box` px square around the reel click |
| `bite_ms` | The reel click minus `-reaction` |
| `empty` | From 1 s after a reel to the next cast, if at least 1 s long |

A cast with no reel, a reel within the first 2 s, and a click outside the frame are skipped and listed. Every labelled cast has a bite, so false bites can only count against early triggers.

`detecteval -export <new dir> dir...` saves each cast's watched area, from the cast until the reel, as `<dataset>/cast-NNN/<ms>.png`. `index.jsonl` has one row per crop with `file`, `dataset`, `cast`, `ms`, `box`, `reel_ms` and the estimated `bite_ms`.
