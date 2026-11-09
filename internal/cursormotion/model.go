// Package cursormotion adapts reviewed cursor paths to a new start and target.
package cursormotion

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"math"
	"math/rand/v2"
	"os"
	"time"
)

// Version identifies the cursor profile format.
const Version = 1

// Session identifies the data and annotations used to extract movement.
type Session struct {
	Name         string `json:"name"`
	TraceSHA256  string `json:"trace_sha256"`
	ReviewSHA256 string `json:"review_sha256"`
	Paths        int    `json:"paths"`
}

// Point locates a waypoint in units of start-to-click distance, along and
// perpendicular to that direction.
type Point struct {
	US     int64   `json:"us"`
	Along  float64 `json:"along"`
	Across float64 `json:"across"`
}

// Settle describes a recorded pause and final correction after the approach.
type Settle struct {
	PauseUS    int64 `json:"pause_us"`
	DurationUS int64 `json:"duration_us"`
}

// Sample is a reviewed approach, expressed relative to the actual reel point.
type Sample struct {
	Distance float64 `json:"distance_px"`
	Points   []Point `json:"points"`
	Settle   *Settle `json:"settle,omitempty"`
}

// Model stores the frozen training paths and the whole-session split.
type Model struct {
	Version  int       `json:"version"`
	Training []Session `json:"training"`
	Holdout  []Session `json:"holdout"`
	Samples  []Sample  `json:"samples"`
}

// Valid checks the bounds and ordering needed by the path planner.
func (s Sample) Valid() bool {
	if math.IsNaN(s.Distance) || math.IsInf(s.Distance, 0) || s.Distance < 20 ||
		len(s.Points) < 3 || len(s.Points) > 128 || s.Points[0] != (Point{}) {
		return false
	}
	for i, p := range s.Points {
		if math.IsNaN(p.Along) || math.IsNaN(p.Across) || math.Abs(p.Along) > 3 || math.Abs(p.Across) > 2 ||
			i > 0 && (p.US <= s.Points[i-1].US || p.US > 1_500_000) {
			return false
		}
	}
	return s.Settle == nil || s.Settle.PauseUS >= 50_000 && s.Settle.PauseUS <= 1_500_000 &&
		s.Settle.DurationUS >= 10_000 && s.Settle.DurationUS <= 250_000
}

// Load reads and validates the entire profile before any input is enabled.
func Load(path string) (Model, error) {
	var m Model
	raw, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, err
	}
	if m.Version != Version || len(m.Samples) == 0 {
		return m, errors.New("unsupported or empty cursor model")
	}
	for i, s := range m.Samples {
		if !s.Valid() {
			return m, fmt.Errorf("invalid cursor sample %d", i)
		}
	}
	return m, nil
}

// Planner returns a seeded planner for one caller. Every point stays inside
// bounds and the final point is exactly target. A settling correction is
// eligible only when the caller can interrupt it before a reel.
func (m Model) Planner(seed uint64) Planner {
	rng := rand.New(rand.NewPCG(seed, 1))
	return func(start, target image.Point, bounds image.Rectangle, settle bool) []Step {
		if !start.In(bounds) || !target.In(bounds) {
			return nil
		}
		dx, dy := float64(target.X-start.X), float64(target.Y-start.Y)
		distance := math.Hypot(dx, dy)
		if distance < 4 {
			return []Step{{Point: target}}
		}
		// Weight by relative distance so a short reach rarely draws a path
		// observed on a much longer move.
		weights := make([]float64, len(m.Samples))
		best := math.Inf(1)
		for i, sample := range m.Samples {
			d := math.Log(distance / sample.Distance)
			weights[i] = d * d
			best = min(best, weights[i])
		}
		var total float64
		for i, w := range weights {
			weights[i] = math.Exp((best - w) / .125)
			total += weights[i]
		}
		draw, chosen := rng.Float64()*total, 0
		for i, w := range weights {
			draw -= w
			if w > 0 {
				chosen = i
			}
			if draw < 0 {
				break
			}
		}
		s := m.Samples[chosen]
		end := s.Points[len(s.Points)-1]
		duration := time.Duration(math.Round(float64(end.US)*math.Sqrt(distance/s.Distance))) * time.Microsecond
		duration = max(50*time.Millisecond, min(duration, 850*time.Millisecond))
		steps := make([]Step, 0, int(duration/(16*time.Millisecond))+2)
		project := func(along, across float64) image.Point {
			return image.Pt(
				max(bounds.Min.X, min(bounds.Max.X-1, start.X+int(math.Round(along*dx-across*dy)))),
				max(bounds.Min.Y, min(bounds.Max.Y-1, start.Y+int(math.Round(along*dy+across*dx)))),
			)
		}
		endAlong, endAcross := 1.0, 0.0
		rest := target
		if settle && s.Settle != nil && rng.IntN(3) == 0 {
			// Retain the recorded direction, with a small bound so a pause
			// never leaves the pointer far from the bobber.
			x, y := end.Along-1, end.Across
			scale := min(1, 8/max(1, distance*math.Hypot(x, y)))
			if p := project(1+x*scale, y*scale); p != target {
				rest, endAlong, endAcross = p, 1+x*scale, y*scale
			}
		}
		idx := 1
		last := start
		// Divide the duration evenly to avoid a very short final interval.
		intervals := (duration + 8*time.Millisecond) / (16 * time.Millisecond)
		for i := time.Duration(1); i < intervals; i++ {
			// Vary interior dispatch times while keeping the path's
			// total duration and leaving space on either side.
			jitter := time.Duration(rng.IntN(4001)-2000) * time.Microsecond
			at := duration*i/intervals + jitter
			progress := float64(at) / float64(duration)
			us := int64(float64(end.US) * progress)
			for idx < len(s.Points)-1 && s.Points[idx].US < us {
				idx++
			}
			a, b := s.Points[idx-1], s.Points[idx]
			t := float64(us-a.US) / float64(b.US-a.US)
			// Bring the recorded endpoint onto the destination without a
			// last-step jump. The correction eases in and out.
			blend := progress * progress * (3 - 2*progress)
			p := project(a.Along+(b.Along-a.Along)*t+blend*(endAlong-end.Along),
				a.Across+(b.Across-a.Across)*t+blend*(endAcross-end.Across))
			if p != last {
				steps = append(steps, Step{After: at, Point: p})
				last = p
			}
		}
		if rest != target {
			steps = append(steps, Step{After: duration, Point: rest})
			duration += time.Duration(s.Settle.PauseUS) * time.Microsecond
			correction := time.Duration(s.Settle.DurationUS) * time.Microsecond
			last = rest
			for at := 16 * time.Millisecond; at < correction; at += 16 * time.Millisecond {
				t := float64(at) / float64(correction)
				p := image.Pt(rest.X+int(math.Round(float64(target.X-rest.X)*t)),
					rest.Y+int(math.Round(float64(target.Y-rest.Y)*t)))
				if p != last && p != target {
					steps = append(steps, Step{After: duration + at, Point: p})
					last = p
				}
			}
			duration += correction
		}
		return append(steps, Step{After: duration, Point: target})
	}
}

// Planner maps an observed path to a new reach inside bounds.
type Planner func(start, target image.Point, bounds image.Rectangle, settle bool) []Step

// Step places the pointer at Point after the given time from the path start.
type Step struct {
	After time.Duration
	Point image.Point
}
