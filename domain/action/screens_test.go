package action

import (
	"image"
	"testing"
)

func TestNumberScreensPrimaryFirstThenLeftToRight(t *testing.T) {
	rects := []image.Rectangle{
		image.Rect(3440, 0, 5360, 1080), // right monitor
		image.Rect(0, 0, 3440, 1440),    // primary
		image.Rect(-1920, 148, 0, 1228), // left monitor
	}
	got := numberScreens(rects, 1)
	wantX := []int{0, -1920, 3440}
	for i, s := range got {
		if s.Number != i+1 || s.X != wantX[i] {
			t.Fatalf("screen %d = %+v, want X %d", i+1, s, wantX[i])
		}
	}
	if !got[0].Primary || got[1].Primary {
		t.Fatalf("primary flag wrong: %+v", got)
	}
	if s, ok := ScreenByNumber(got, 2); !ok || s.W != 1920 || s.H != 1080 {
		t.Fatalf("ScreenByNumber(2) = %+v, %v", s, ok)
	}
	if _, ok := ScreenByNumber(got, 4); ok {
		t.Fatalf("screen 4 should not exist")
	}
}
