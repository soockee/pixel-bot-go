package main

import (
	"image"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/internal/cursormotion"
	"github.com/soocke/pixel-bot-go/internal/playtrace"
)

// Default paths written as a dry run writes them come back as one reach each,
// from where the pointer rested, with their planned length and shape.
func TestReadTraceReaches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.jsonl")
	utc := time.Now().UTC()
	w, err := playtrace.Create(path, playtrace.Header{Source: playtrace.AppDryRun, Clock: playtrace.Monotonic, StartedUTC: &utc})
	if err != nil {
		t.Fatal(err)
	}
	bounds := image.Rect(0, 0, 800, 600)
	points := []image.Point{{100, 500}, {600, 150}, {200, 300}, {700, 450}}
	var at int64
	var planned []time.Duration
	for i := 1; i < len(points); i++ {
		at += 500_000 // a pause between reaches
		path := cursormotion.DefaultPath(points[i-1], points[i], bounds, false)
		planned = append(planned, path[len(path)-1].After)
		for _, step := range path {
			p := step.Point
			ev := playtrace.Event{Type: "move", ElapsedUS: at + step.After.Microseconds(),
				Position: &playtrace.Position{X: int32(p.X), Y: int32(p.Y), Width: 800, Height: 600}}
			if err := w.Write(ev); err != nil {
				t.Fatal(err)
			}
		}
		at += planned[len(planned)-1].Microseconds()
	}
	if err := w.Close(playtrace.End{ElapsedUS: at, Status: "complete"}); err != nil {
		t.Fatal(err)
	}

	metrics, shapes, err := readTraceReaches(path)
	if err != nil {
		t.Fatal(err)
	}
	// The first path's start is not in the trace, so it is left out.
	if len(metrics) != 2 || len(shapes) != 2 {
		t.Fatalf("got %d reaches, want 2", len(metrics))
	}
	for i, m := range metrics {
		want := float64(planned[i+1]) / float64(time.Millisecond)
		if math.Abs(m.DurationMS-want) > 15 || m.TravelRatio < 1 || m.TravelRatio > 3 {
			t.Errorf("reach %d = %+v, want about %.0f ms and a travel ratio from 1 to 3", i, m, want)
		}
	}
}
