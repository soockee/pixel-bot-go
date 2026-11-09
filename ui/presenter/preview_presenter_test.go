package presenter

import (
	"image"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/domain/fishing"
)

type mockPreviewSource struct{ preview *fishing.Preview }

func (s *mockPreviewSource) Preview() *fishing.Preview { return s.preview }

type mockPreviewView struct{ captures, rois int }

func (v *mockPreviewView) UpdateCapture(*image.RGBA) { v.captures++ }
func (v *mockPreviewView) UpdateDetection(img *image.RGBA, _ image.Rectangle, _ *image.RGBA, _ image.Rectangle) {
	if img != nil {
		v.rois++
	}
}

func TestPreviewPresenterLimitsTheUpdateRate(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 1, 1))
	source := &mockPreviewSource{}
	view := &mockPreviewView{}
	p := NewPreviewPresenter(source, view)
	start := time.Now()

	p.Tick(start)
	if view.captures != 0 {
		t.Fatal("updated the view without a preview")
	}

	source.preview = &fishing.Preview{Frame: frame}
	p.Tick(start)
	p.Tick(start.Add(previewInterval / 2))
	if view.captures != 1 || view.rois != 0 {
		t.Fatalf("captures=%d rois=%d, want 1 and 0", view.captures, view.rois)
	}

	// Watching keeps the capture and sends only the live box.
	source.preview = &fishing.Preview{Frame: frame, ROI: frame}
	p.Tick(start.Add(previewInterval))
	if view.captures != 1 || view.rois != 1 {
		t.Fatalf("captures=%d rois=%d, want 1 and 1", view.captures, view.rois)
	}
}
