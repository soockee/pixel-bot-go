package images

import (
	"image"
	"testing"
)

func TestExtractROI_CentersAndClamps(t *testing.T) {
	// frame: 100x100 image
	frame := image.NewRGBA(image.Rect(0, 0, 100, 100))
	roi, rect, err := ExtractROI(frame, 50, 50, 40)
	if err != nil || roi == nil {
		t.Fatalf("expected ROI, got err=%v", err)
	}
	if rect.Dx() != 40 || rect.Dy() != 40 {
		t.Fatalf("expected 40x40, got %dx%d", rect.Dx(), rect.Dy())
	}
	if rect.Min.X != 30 || rect.Min.Y != 30 {
		t.Fatalf("unexpected rect origin %v", rect.Min)
	}
}

func TestExtractROI_ClampsNearEdge(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 20, 20))
	roi, rect, err := ExtractROI(frame, 2, 2, 10)
	if err != nil || roi == nil {
		t.Fatalf("roi error: %v", err)
	}
	if rect.Min.X != 0 || rect.Min.Y != 0 {
		t.Fatalf("expected clamp to 0,0 got %v", rect.Min)
	}
	if rect.Max.X > 20 || rect.Max.Y > 20 {
		t.Fatalf("rect exceeds frame bounds: %v", rect)
	}
}

func TestExtractROI_SizeAdjustedWhenTooLarge(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 30, 30))
	// requested size larger than frame
	roi, rect, _ := ExtractROI(frame, 5, 5, 50)
	if roi == nil {
		t.Fatalf("nil roi")
	}
	if rect.Max.X > 30 || rect.Max.Y > 30 {
		t.Fatalf("rect beyond frame: %v", rect)
	}
}

func TestExtractROI_MinSize(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 10, 10))
	// requested size zero -> expect minimum 1x1 ROI
	roi, rect, _ := ExtractROI(frame, 0, 0, 0)
	if roi == nil {
		t.Fatalf("nil roi")
	}
	if rect.Dx() != 1 || rect.Dy() != 1 {
		t.Fatalf("expected 1x1 got %dx%d", rect.Dx(), rect.Dy())
	}
}
