package desktop

import (
	"bufio"
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

// summaryCasts is how many of the last casts the summary lists one by one.
const summaryCasts = 8

// RunSummary condenses the newest kept log that has casts in it into a few
// lines for a problem report: how the runs went, why they stopped, the board,
// and the last casts with their splashes and sounds. A person who restarted
// the app after the problem still reports it, from the log before. It is ""
// when no kept log has a cast.
func RunSummary(path string) string {
	for n := range config.LogsKept {
		if s := summarize(config.KeptLogPath(path, n)); s != "" {
			return s
		}
	}
	return ""
}

// summaryCast is one cast, from the lock on the bobber to the end of watching.
type summaryCast struct {
	at, end   time.Time
	splashes  int
	share     float64 // the largest splash share
	offSounds int     // sounds that came outside a splash
	bite      string  // "" or what the bite detection logged
	result    string
}

func summarize(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	var (
		first, last             time.Time
		starts, casts, ranOut   int
		bites, halts, noBiteSnd int
		pointerFails            int
		lastTrail, board        string
		outcomes, misses, stops = map[string]int{}, map[string]int{}, map[string]int{}
		watched                 []*summaryCast
		cur                     *summaryCast
	)
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64*1024), 1024*1024)
	for scan.Scan() {
		var e map[string]any
		if json.Unmarshal(scan.Bytes(), &e) != nil {
			continue
		}
		at, _ := time.Parse(time.RFC3339Nano, text(e, "time"))
		if first.IsZero() {
			first = at
		}
		last = at
		msg := text(e, "msg")
		switch {
		case msg == "ESP32 keyboard ready":
			starts++
			board = fields(e)
		case msg == "cast action executed":
			casts++
		case msg == "found blobber":
			cur = &summaryCast{at: at}
			watched = append(watched, cur)
		case msg == "splash seen" && cur != nil:
			cur.splashes++
			cur.share = math.Max(cur.share, number(e, "share"))
		case msg == "sound outside the splash" && cur != nil:
			cur.offSounds++
		case msg == "bite detected":
			bites++
			if cur != nil {
				cur.bite = "bite without sound"
				if ms, ok := e["sound_after_splash_ms"].(float64); ok {
					cur.bite = fmt.Sprintf("bite (sound %+d ms after the splash)", int(ms))
				}
			}
		case msg == "reel missed":
			misses[text(e, "reason")]++
			if cur != nil {
				cur.result = "planned miss: " + text(e, "reason")
			}
		case msg == "fishing outcome":
			outcomes[text(e, "outcome")]++
			if cur != nil {
				cur.result = text(e, "outcome")
			}
		case msg == "fishing state transition" && text(e, "from") == "monitoring":
			if cur != nil && cur.end.IsZero() {
				cur.end = at
				if text(e, "to") != "reeling" {
					cur.result = "ended in " + text(e, "to")
				}
			}
			if text(e, "to") == "casting" {
				ranOut++
				if cur != nil {
					cur.result = "ran out"
				}
			}
		case strings.HasPrefix(msg, "halting: "):
			stops[strings.TrimPrefix(msg, "halting: ")]++
		case msg == "ESP32 pointer move failed":
			pointerFails++
			lastTrail = text(e, "error")
		case strings.HasPrefix(msg, "no bite sound after splashes"):
			noBiteSnd++
		}
		if msg == "fishing state transition" && text(e, "to") == "halt" {
			halts++
		}
	}
	if casts == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Log %s to %s (%s), fishing started %d times.\n",
		first.Format("15:04:05"), last.Format("15:04:05"), last.Sub(first).Round(time.Minute), starts)
	if board != "" {
		fmt.Fprintf(&b, "Board: %s\n", board)
	}
	fmt.Fprintf(&b, "Casts %d: bites %d, ran out without a bite %d.", casts, bites, ranOut)
	if len(outcomes) > 0 {
		fmt.Fprintf(&b, " Outcomes: %s.", counts(outcomes))
	}
	if len(misses) > 0 {
		fmt.Fprintf(&b, " Planned misses: %s.", counts(misses))
	}
	b.WriteString("\n")
	reasons := 0
	for _, n := range stops {
		reasons += n
	}
	if reasons > 0 {
		fmt.Fprintf(&b, "Stopped by itself: %s.\n", counts(stops))
	}
	if other := halts - reasons; other > 0 {
		fmt.Fprintf(&b, "Stopped by Stop, or by the game losing focus: %d.\n", other)
	}
	if noBiteSnd > 0 {
		fmt.Fprintf(&b, "Splashes came without the bite sound %d times; the game's sound effects may be off.\n", noBiteSnd)
	}
	if pointerFails > 0 {
		fmt.Fprintf(&b, "Pointer moves failed %d times, the last: %s\n", pointerFails, cut(lastTrail, 300))
	}
	b.WriteString("Last casts (lock, watched, splashes and largest share, sounds outside a splash, result):\n")
	for _, c := range watched[max(0, len(watched)-summaryCasts):] {
		watched := "still watching"
		if !c.end.IsZero() {
			watched = fmt.Sprintf("%.1fs", c.end.Sub(c.at).Seconds())
		}
		splashes := fmt.Sprint(c.splashes)
		if c.splashes > 0 {
			splashes += fmt.Sprintf(" (%.2f)", c.share)
		}
		result := cmp.Or(c.result, "-")
		if c.bite != "" {
			result = c.bite + ", " + result
		}
		fmt.Fprintf(&b, "  %s %s, splashes %s, sounds %d, %s\n", c.at.Format("15:04:05"), watched, splashes, c.offSounds, result)
	}
	return b.String()
}

// text is the string field k of a log entry, or "".
func text(e map[string]any, k string) string {
	s, _ := e[k].(string)
	return s
}

// number is the number field k of a log entry, or 0.
func number(e map[string]any, k string) float64 {
	n, _ := e[k].(float64)
	return n
}

// fields lists a log entry's own fields as "name=value", sorted.
func fields(e map[string]any) string {
	var parts []string
	for _, k := range slices.Sorted(maps.Keys(e)) {
		if k != "time" && k != "level" && k != "msg" {
			parts = append(parts, fmt.Sprintf("%s=%v", k, e[k]))
		}
	}
	return strings.Join(parts, " ")
}

// counts lists "n× what" by count, most first.
func counts(m map[string]int) string {
	keys := slices.SortedFunc(maps.Keys(m), func(a, b string) int { return cmp.Or(m[b]-m[a], strings.Compare(a, b)) })
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%d× %s", m[k], k)
	}
	return strings.Join(parts, "; ")
}

// cut shortens s to at most n bytes.
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "") + "…"
}
