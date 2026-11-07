package capture

import (
	"image"
	"testing"
)

func TestNextRequiresARegion(t *testing.T) {
	empty := image.Rectangle{}
	for name, region := range map[string]*image.Rectangle{"nil": nil, "empty": &empty} {
		t.Run(name, func(t *testing.T) {
			src := Source{Region: func() *image.Rectangle { return region }}
			if _, err := src.Next(); err == nil {
				t.Error("want an error")
			}
		})
	}
}

// A region hanging off the desktop must report where the image really starts,
// or every coordinate derived from it is wrong.
func TestNextReportsTheClippedOrigin(t *testing.T) {
	left := int(getSystemMetric(smXVirtual))
	top := int(getSystemMetric(smYVirtual))
	sel := image.Rect(left-10, top-10, left+20, top+20)
	frame, err := Source{Region: func() *image.Rectangle { return &sel }}.Next()
	if err != nil {
		t.Skipf("no desktop to capture: %v", err)
	}
	if want := image.Pt(left, top); frame.Origin != want {
		t.Errorf("Origin = %v, want %v", frame.Origin, want)
	}
	if got := frame.Image.Bounds().Size(); got != image.Pt(20, 20) {
		t.Errorf("size = %v, want 20x20", got)
	}
}
