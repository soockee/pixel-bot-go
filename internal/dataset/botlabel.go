package dataset

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// BotLogFile holds what the bot did while a recording ran (recorder.BotLog):
// one JSON object per log record, with "ms" on the recording's clock.
const BotLogFile = "bot.jsonl"

// botRecord is the part of a bot.jsonl row the labels need.
type botRecord struct {
	MS      int64  `json:"ms"`
	Msg     string `json:"msg"`
	X       *int   `json:"x"`
	Y       *int   `json:"y"`
	Outcome string `json:"outcome"`
	GapMS   *int64 `json:"sound_after_splash_ms"`
	Reason  string `json:"reason"`
}

// botCast is one cast as the bot logged it.
type botCast struct {
	castMS, lockMS, detectMS, reelMS int64
	gapMS                            int64
	x, y                             int
	locked, detected, reeled         bool
	outcome, missed                  string
}

// BotLabel labels a recording of a bot run from its bot.jsonl, where Bobber
// Marker said how each reel ended. A reel that caught a fish or let it get
// away followed a real bite: the bite is labelled at the splash, the
// detection less the splash-to-sound gap it logged. A reel with no fish
// hooked followed a false bite: the cast is labelled with none. Casts that
// ran out, a deliberate miss, or a reel without an outcome are not known and
// are listed as skipped, so a person can add them by hand. Each cast spans
// from the lock to the reel. The bobber is a boxPx square around the locked
// point, which bot.jsonl gives in screen coordinates; recordings of a game
// window at the screen's origin need no shift.
func BotLabel(dir string, boxPx int) (Labels, []string, error) {
	var labels Labels
	f, err := os.Open(filepath.Join(dir, BotLogFile))
	if err != nil {
		return labels, nil, fmt.Errorf("record a bot run to get bot labels: %w", err)
	}
	defer f.Close()
	var casts []*botCast
	var cur *botCast
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64*1024), 1024*1024)
	for scan.Scan() {
		var r botRecord
		if json.Unmarshal(scan.Bytes(), &r) != nil {
			continue
		}
		switch r.Msg {
		case "cast action executed":
			cur = &botCast{castMS: r.MS}
			casts = append(casts, cur)
		case "found blobber":
			if cur != nil && !cur.locked && r.X != nil && r.Y != nil {
				cur.lockMS, cur.x, cur.y, cur.locked = r.MS, *r.X, *r.Y, true
			}
		case "bite detected":
			if cur != nil && cur.locked && !cur.detected {
				cur.detectMS, cur.detected = r.MS, true
				if r.GapMS != nil {
					cur.gapMS = *r.GapMS
				}
			}
		case "reel action executed":
			if cur != nil && cur.detected && !cur.reeled {
				cur.reelMS, cur.reeled = r.MS, true
			}
		case "reel missed":
			if cur != nil {
				cur.missed = r.Reason
			}
		case "fishing outcome":
			// The outcome may come after the next cast began: it belongs to
			// the latest reel still waiting for one.
			for i := len(casts) - 1; i >= 0; i-- {
				if c := casts[i]; c.reeled && c.outcome == "" {
					c.outcome = r.Outcome
					break
				}
			}
		}
	}
	if err := scan.Err(); err != nil {
		return labels, nil, err
	}
	var skipped []string
	for _, c := range casts {
		switch {
		case !c.locked:
			// The bobber was never found; nothing to watch.
		case c.missed != "":
			skipped = append(skipped, fmt.Sprintf("cast at %d ms: deliberate miss (%s), a bite of unknown time", c.castMS, c.missed))
		case !c.reeled:
			skipped = append(skipped, fmt.Sprintf("cast at %d ms: no reel; check by hand whether a bite came", c.castMS))
		case c.outcome == "":
			skipped = append(skipped, fmt.Sprintf("cast at %d ms: reel without an outcome", c.castMS))
		default:
			cast := Cast{
				Span:   Span{StartMS: c.lockMS, EndMS: c.reelMS},
				Bobber: Rect{X: c.x - boxPx/2, Y: c.y - boxPx/2, W: boxPx, H: boxPx},
			}
			if c.outcome != "not_hooked" {
				bite := c.detectMS - c.gapMS
				cast.BiteMS = &bite
			}
			labels.Casts = append(labels.Casts, cast)
		}
	}
	if len(labels.Casts) == 0 {
		return labels, skipped, fmt.Errorf("no cast in %s has an outcome; record with Bobber Marker 1.5.1 or later", BotLogFile)
	}
	return labels, skipped, labels.check()
}
