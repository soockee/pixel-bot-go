package app

import (
	"image"
	"math"
	"testing"
)

func TestPillRectAndPlaceAreInverse(t *testing.T) {
	// A game window that is not at the origin, as on a second monitor.
	area := image.Rect(-1920, 148, 0, 1228)
	for _, p := range [][2]float64{{0.5, 0.01}, {0.2, 0.7}, {0.9, 0.5}, {0, 0}, {1, 1}} {
		pill := pillRect(area, p[0], p[1], 380, 44)
		x, y, ok := pillPlace(pill, area)
		if !ok || math.Abs(x-p[0]) > 0.001 || math.Abs(y-p[1]) > 0.001 {
			t.Errorf("place %v -> %v -> (%v, %v, %v)", p, pill, x, y, ok)
		}
		if !pill.In(area) {
			t.Errorf("pill %v leaves the area %v", pill, area)
		}
	}
}

func TestPillStaysInsideTheArea(t *testing.T) {
	area := image.Rect(100, 100, 1100, 700)
	for name, tc := range map[string]struct {
		x, y float64
		want image.Rectangle
	}{
		"top left":     {0, 0, image.Rect(100, 100, 480, 144)},
		"top right":    {1, 0, image.Rect(720, 100, 1100, 144)},
		"bottom":       {0.5, 1, image.Rect(410, 656, 790, 700)}, // centred on x = 600
		"past the end": {1.5, -1, image.Rect(720, 100, 1100, 144)},
	} {
		if got := pillRect(area, tc.x, tc.y, 380, 44); got != tc.want {
			t.Errorf("%s: pillRect = %v, want %v", name, got, tc.want)
		}
	}
}

// The default place, the top centre, must stay the top centre: the pill sits in
// the middle of the free space.
func TestDefaultPlaceIsTheTopCentre(t *testing.T) {
	area := image.Rect(0, 0, 1920, 1080)
	pill := pillRect(area, 0.5, 0.01, 380, 44)
	if centre := (pill.Min.X + pill.Max.X) / 2; centre != 960 {
		t.Errorf("pill centre = %d, want the middle of the window", centre)
	}
	if pill.Min.Y > 12 {
		t.Errorf("pill top = %d, want it close to the top edge", pill.Min.Y)
	}
}

func TestPillPlaceCorrectsAPillDraggedOutside(t *testing.T) {
	area := image.Rect(0, 0, 1000, 600)
	// Dragged mostly off the right and bottom edges: pulled back to the corner.
	x, y, ok := pillPlace(image.Rect(900, 590, 1280, 634), area)
	if !ok || x != 1 || y != 1 {
		t.Fatalf("place = (%v, %v, %v), want the bottom right corner (1, 1)", x, y, ok)
	}
}

func TestPillPlaceNeedsAnAreaAndAPill(t *testing.T) {
	pill := image.Rect(0, 0, 380, 44)
	if _, _, ok := pillPlace(pill, image.Rectangle{}); ok {
		t.Error("a place without a game window")
	}
	if _, _, ok := pillPlace(image.Rectangle{}, image.Rect(0, 0, 100, 100)); ok {
		t.Error("a place without a pill")
	}
}

func TestPillWiderThanTheWindowHasNoFreeSpace(t *testing.T) {
	area := image.Rect(0, 0, 300, 200) // narrower than the pill
	pill := pillRect(area, 0.7, 0.5, 380, 44)
	if pill.Min.X != 0 {
		t.Errorf("pill left = %d, want it at the window's left edge", pill.Min.X)
	}
	if x, _, ok := pillPlace(pill, area); !ok || x != 0 {
		t.Errorf("place x = %v, %v; want 0 when there is no free width", x, ok)
	}
}
