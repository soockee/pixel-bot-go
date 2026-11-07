package presenter

import (
	"image"
	"time"

	"github.com/soocke/pixel-bot-go/domain/fishing"
)

// previewInterval limits preview updates, which are costly on the Tk thread.
const previewInterval = 100 * time.Millisecond

// PreviewSource supplies the latest pipeline images.
type PreviewSource interface {
	Preview() *fishing.Preview
}

// PreviewView shows the capture and the region around the bobber.
type PreviewView interface {
	UpdateCapture(img *image.RGBA)
	UpdateDetection(img *image.RGBA)
}

// PreviewPresenter copies pipeline images to the view at a low rate.
type PreviewPresenter struct {
	source PreviewSource
	view   PreviewView
	last   time.Time
}

// NewPreviewPresenter returns a PreviewPresenter.
func NewPreviewPresenter(source PreviewSource, view PreviewView) *PreviewPresenter {
	return &PreviewPresenter{source: source, view: view}
}

// Tick updates the view when an update is due and images are available.
func (p *PreviewPresenter) Tick(now time.Time) {
	if now.Sub(p.last) < previewInterval {
		return
	}
	preview := p.source.Preview()
	if preview == nil {
		return
	}
	p.last = now
	p.view.UpdateCapture(preview.Frame)
	if preview.ROI != nil {
		p.view.UpdateDetection(preview.ROI)
	}
}
