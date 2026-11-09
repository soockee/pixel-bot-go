package main

import (
	"image"
	"math"
	"time"

	"github.com/soocke/pixel-bot-go/internal/cursormotion"
)

const reachSamples = 32

type reachShape [reachSamples][2]float64

type reachAudit struct {
	Paths             int      `json:"paths"`
	Comparisons       int      `json:"paths_with_history"`
	MeanNearest       *float64 `json:"mean_nearest_dtw,omitempty"`
	MeanPeakSpeed     *float64 `json:"mean_peak_reaches_per_second,omitempty"`
	MeanPeakSpeedTime *float64 `json:"mean_peak_speed_time_fraction,omitempty"`
	MeanStepMS        *float64 `json:"mean_step_interval_ms,omitempty"`
}

type sessionReachAudit struct {
	Session            string       `json:"session"`
	DirectInstantMoves int          `json:"direct_instant_moves"`
	Human              reachAudit   `json:"human"`
	Fitted             []reachAudit `json:"fitted_by_seed"`
	Default            []reachAudit `json:"default_by_trial"` // the path live runs use without a profile
}

func auditFittedReaches(examples []cursorExample, plan cursormotion.Planner) reachAudit {
	var history reachHistory
	for _, e := range examples {
		steps := plan(e.start, e.target, e.bounds, false)
		history.observe(plannedReach(e.start, e.target, steps))
	}
	return history.result()
}

type reachHistory struct {
	shapes                 []reachShape
	paths, comparisons     int
	nearest, speed, peakAt float64
	elapsedUS, steps       int64
}

func (h *reachHistory) observe(points []cursormotion.Point) {
	h.paths++
	if len(points) < 2 || points[len(points)-1].US <= 0 {
		return
	}
	duration := float64(points[len(points)-1].US)
	var shape reachShape
	var peak, peakAt float64
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		speed := math.Hypot(b.Along-a.Along, b.Across-a.Across) * 1e6 / float64(b.US-a.US)
		if speed > peak {
			peak, peakAt = speed, (float64(a.US)+float64(b.US))/(2*duration)
		}
	}
	right := 1
	for i := range shape {
		at := duration * float64(i) / (reachSamples - 1)
		for right < len(points)-1 && float64(points[right].US) < at {
			right++
		}
		a, b := points[right-1], points[right]
		fraction := (at - float64(a.US)) / float64(b.US-a.US)
		shape[i] = [2]float64{a.Along + fraction*(b.Along-a.Along), a.Across + fraction*(b.Across-a.Across)}
	}
	if len(h.shapes) != 0 {
		nearest := math.Inf(1)
		for _, previous := range h.shapes {
			nearest = min(nearest, reachDTW(previous, shape))
		}
		h.nearest += nearest
		h.comparisons++
	}
	const historyLimit = 16
	if len(h.shapes) == historyLimit {
		h.shapes = h.shapes[1:]
	}
	h.shapes = append(h.shapes, shape)
	h.speed += peak
	h.peakAt += peakAt
	h.elapsedUS += points[len(points)-1].US
	h.steps += int64(len(points) - 1)
}

func (h reachHistory) result() reachAudit {
	r := reachAudit{Paths: h.paths, Comparisons: h.comparisons}
	if h.comparisons > 0 {
		nearest := h.nearest / float64(h.comparisons)
		r.MeanNearest = &nearest
	}
	if n := h.comparisons + 1; h.steps > 0 {
		speed, peakAt := h.speed/float64(n), h.peakAt/float64(n)
		stepMS := float64(h.elapsedUS) / float64(h.steps) / 1000
		r.MeanPeakSpeed, r.MeanPeakSpeedTime, r.MeanStepMS = &speed, &peakAt, &stepMS
	}
	return r
}

// reachDTW allows a local shift of up to four samples on the normalized clock.
// The band retains information about braking and the order of corrections.
func reachDTW(a, b reachShape) float64 {
	var previous, current [reachSamples + 1]float64
	for j := 1; j <= reachSamples; j++ {
		previous[j] = math.Inf(1)
	}
	for i, p := range a {
		for j := range current {
			current[j] = math.Inf(1)
		}
		for j := max(0, i-4); j <= min(reachSamples-1, i+4); j++ {
			q := b[j]
			current[j+1] = math.Hypot(p[0]-q[0], p[1]-q[1]) + min(previous[j], previous[j+1], current[j])
		}
		previous, current = current, previous
	}
	return previous[reachSamples] / reachSamples
}

func plannedReach(start, target image.Point, steps []cursormotion.Step) []cursormotion.Point {
	dx, dy := float64(target.X-start.X), float64(target.Y-start.Y)
	d2 := dx*dx + dy*dy
	points := make([]cursormotion.Point, 1, len(steps)+1)
	for _, step := range steps {
		us := int64(step.After / time.Microsecond)
		if us <= points[len(points)-1].US {
			continue
		}
		x, y := float64(step.Point.X-start.X), float64(step.Point.Y-start.Y)
		points = append(points, cursormotion.Point{US: us, Along: (dx*x + dy*y) / d2, Across: (dx*y - dy*x) / d2})
	}
	return points
}
