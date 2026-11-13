package main

import (
	"image"
	"os"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/domain/capture"
)

func TestStreamSpeed(t *testing.T) {
	procSetDPI.Call(^uintptr(3))
	w, err := byPID("World of Warcraft", 40556)
	if err != nil {
		t.Skip(err)
	}
	t.Log("game in front:", w.Focused())
	start := time.Now()
	s, err := capture.OpenWindowStream(w)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	t.Log("open:", time.Since(start))
	area, _ := w.Area()
	f, err := s.Grab(area)
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll("../../datasets/bgprobe/wgc", 0o755)
	save("../../datasets/bgprobe/wgc", "full.png", f.Image)
	t.Logf("full frame %v at %v, %v old", f.Image.Bounds().Size(), f.Origin, time.Since(f.CapturedAt))
	box := image.Rectangle{Min: area.Min.Add(image.Pt(1600, 400)), Max: area.Min.Add(image.Pt(1720, 520))}
	for _, r := range []image.Rectangle{area, box} {
		start := time.Now()
		seen := map[time.Time]bool{}
		var oldest, newest time.Duration = 0, time.Hour
		for range 60 {
			f, err := s.Grab(r)
			if err != nil {
				t.Fatal(err)
			}
			age := time.Since(f.CapturedAt)
			oldest, newest = max(oldest, age), min(newest, age)
			seen[f.CapturedAt] = true
			time.Sleep(10 * time.Millisecond)
		}
		per := (time.Since(start) - 60*10*time.Millisecond) / 60
		t.Logf("Grab %v: %v per grab, %d distinct frames in %v, age %v to %v", r.Size(), per, len(seen), time.Since(start).Round(time.Millisecond), newest, oldest)
	}
}
