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
	// UpdateDetection shows the box watched for a bite and the live area around
	// it, each at its place on the capture; nil images: none.
	UpdateDetection(roi *image.RGBA, roiBox image.Rectangle, scene *image.RGBA, sceneBox image.Rectangle)
}

// PreviewPresenter copies pipeline images to the view at a low rate.
type PreviewPresenter struct {
	source PreviewSource
	view   PreviewView
	last   time.Time
	frame  *image.RGBA // the capture last shown
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
	// While a bobber is watched the capture stays the same; only its box is live.
	if preview.Frame != p.frame {
		p.frame = preview.Frame
		p.view.UpdateCapture(preview.Frame)
	}
	p.view.UpdateDetection(preview.ROI, preview.ROIBox, preview.Scene, preview.SceneBox)
}
