package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"slices"
	"time"

	"github.com/soocke/pixel-bot-go/internal/cursormotion"
	"github.com/soocke/pixel-bot-go/internal/playtrace"
)

// traceReachGap ends a planned reach in a dry-run trace: the app sends a
// path's waypoints about 10 ms apart, so a longer pause is a new reach.
const traceReachGap = 60 * time.Millisecond

// cursorSummary describes the reaches of one source in the units of the
// -cursor-fit report, so a dry run can be put next to a human recording.
type cursorSummary struct {
	Source  string        `json:"source"`
	Reaches int           `json:"reaches"`
	Median  motionMetrics `json:"median"`
	Audit   reachAudit    `json:"audit"`
}

// summarize returns the median metrics and the reach audit of reaches.
func summarize(source string, metrics []motionMetrics, shapes [][]cursormotion.Point) cursorSummary {
	s := cursorSummary{Source: source, Reaches: len(metrics)}
	median := func(get func(motionMetrics) float64) float64 {
		if len(metrics) == 0 {
			return 0
		}
		v := make([]float64, len(metrics))
		for i, m := range metrics {
			v[i] = get(m)
		}
		slices.Sort(v)
		return v[len(v)/2]
	}
	s.Median = motionMetrics{
		DurationMS:  median(func(m motionMetrics) float64 { return m.DurationMS }),
		TravelRatio: median(func(m motionMetrics) float64 { return m.TravelRatio }),
		Lateral:     median(func(m motionMetrics) float64 { return m.Lateral }),
	}
	var history reachHistory
	for _, points := range shapes {
		history.observe(points)
	}
	s.Audit = history.result()
	return s
}

// readTraceReaches returns the pointer reaches of an app dry-run trace written
// with -trace-out. A reach runs from where the pointer rested to the last
// waypoint before a pause; the first reach, whose start is unknown, and
// reaches under 20 pixels are left out.
func readTraceReaches(path string) (metrics []motionMetrics, shapes [][]cursormotion.Point, err error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	dec := json.NewDecoder(file)
	var first json.RawMessage
	if err := dec.Decode(&first); err != nil {
		return nil, nil, err
	}
	if _, err := playtrace.DecodeHeader(first); err != nil {
		return nil, nil, err
	}
	var start image.Point
	var known bool
	var burst []cursorPoint
	flush := func() {
		defer func() { burst = nil }()
		if len(burst) == 0 {
			return
		}
		end := burst[len(burst)-1].p
		from, wasKnown := start, known
		start, known = end, true
		if !wasKnown || cursorDistance(from, end) < 20 {
			return
		}
		steps := make([]cursormotion.Step, len(burst))
		for i, p := range burst {
			// The first waypoint is one step after the reach began.
			steps[i] = cursormotion.Step{After: time.Duration(p.us-burst[0].us)*time.Microsecond + 10*time.Millisecond, Point: p.p}
		}
		metrics = append(metrics, measureMotion(from, end, steps))
		shapes = append(shapes, plannedReach(from, end, steps))
	}
	err = playtrace.ReadEvents(dec, func(e playtrace.Event) error {
		if e.Type != "move" || e.Position == nil {
			return nil
		}
		p := cursorPoint{us: e.ElapsedUS, p: image.Pt(int(e.Position.X), int(e.Position.Y))}
		if len(burst) > 0 && time.Duration(p.us-burst[len(burst)-1].us)*time.Microsecond > traceReachGap {
			flush()
		}
		burst = append(burst, p)
		return nil
	})
	flush()
	return metrics, shapes, err
}

// runCursorAudit summarizes the pointer reaches of dry-run traces (files) and
// reviewed recordings (directories), one source each.
func runCursorAudit(paths []string, cast, reel string, out io.Writer) error {
	if len(paths) == 0 {
		return errors.New("-cursor-audit needs app traces or recording directories")
	}
	var report []cursorSummary
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			metrics, shapes, err := readTraceReaches(path)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			report = append(report, summarize(path, metrics, shapes))
			continue
		}
		session, examples, err := readCursorSession(path, cast, reel)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		metrics := make([]motionMetrics, len(examples))
		shapes := make([][]cursormotion.Point, len(examples))
		for i, e := range examples {
			metrics[i], shapes[i] = e.human, e.points
		}
		report = append(report, summarize(session.Name, metrics, shapes))
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}
