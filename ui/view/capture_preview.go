package view

import (
	"image"

	"github.com/soocke/pixel-bot-go/ui/images"

	//lint:ignore ST1001 Dot import is intentional for concise Tk widget DSL builders.
	. "modernc.org/tk9.0"
)

// CapturePreview abstracts the capture frame (full/selection) and detection ROI preview.
// It owns two LabelWidgets and provides methods to update or reset them.
type CapturePreview interface {
	UpdateCapture(img image.Image)
	UpdateDetection(img image.Image)
	Reset()
}

type capturePreview struct {
	captureLabel       *LabelWidget
	detectionLabel     *LabelWidget
	targetW            int
	targetH            int
	prevCapturePhoto   *Img // last Tk photo image instance for capture
	prevDetectionPhoto *Img // last Tk photo image instance for detection
}

// Internal state tracks current preview photos so we can dispose old images
// before replacing them, preventing accumulation of off-screen image data.

// NewCapturePreview creates the preview labels, grids them and returns the view.
// Layout: capture spans columns 0-3; detection ROI sits at column 4 of the provided row.
func NewCapturePreview(row int) CapturePreview {
	placeholder := image.NewRGBA(image.Rect(0, 0, 200, 120))
	pngBytes := images.EncodePNG(placeholder)
	capPhoto := NewPhoto(Data(pngBytes))
	detPhoto := NewPhoto(Data(pngBytes))
	capture := Label(Image(capPhoto), Borderwidth(1), Relief("sunken"))
	detection := Label(Image(detPhoto), Borderwidth(1), Relief("sunken"))
	Grid(capture, Row(row), Column(0), Columnspan(4), Sticky("we"), Padx("0.4m"), Pady("0.4m"))
	Grid(detection, Row(row), Column(4), Columnspan(1), Sticky("we"), Padx("0.4m"), Pady("0.4m"))
	return &capturePreview{captureLabel: capture, detectionLabel: detection, prevCapturePhoto: capPhoto, prevDetectionPhoto: detPhoto}
}

const (
	// Max preview dimensions (reduced to shrink on-screen footprint).
	// Adjust if you need higher detail; scaling is proportional.
	maxPreviewW = 400
	maxPreviewH = 225
)

func (v *capturePreview) UpdateCapture(img image.Image) {
	if v.captureLabel == nil || img == nil {
		return
	}
	// Determine target size (fallback to max constants if unset).
	w, h := v.targetW, v.targetH
	if w <= 0 || h <= 0 {
		w, h = maxPreviewW, maxPreviewH
	}
	// Scale for display only; allocate a fresh scaled image each call.
	scaled := images.ScaleToFit(img, w, h)
	pngBytes := images.EncodePNG(scaled)
	// Replace previous photo to avoid retaining obsolete pixel buffers.
	if v.prevCapturePhoto != nil {
		v.prevCapturePhoto.Delete()
	}
	newPhoto := NewPhoto(Data(pngBytes))
	v.prevCapturePhoto = newPhoto
	v.captureLabel.Configure(Image(newPhoto))
}

func (v *capturePreview) UpdateDetection(img image.Image) {
	if v.detectionLabel == nil || img == nil {
		return
	}
	pngBytes := images.EncodePNG(img)
	if v.prevDetectionPhoto != nil {
		v.prevDetectionPhoto.Delete()
	}
	newPhoto := NewPhoto(Data(pngBytes))
	v.prevDetectionPhoto = newPhoto
	v.detectionLabel.Configure(Image(newPhoto))
}

func (v *capturePreview) Reset() {
	placeholder := image.NewRGBA(image.Rect(0, 0, 200, 120))
	pngBytes := images.EncodePNG(placeholder)
	if v.captureLabel != nil {
		if v.prevCapturePhoto != nil {
			v.prevCapturePhoto.Delete()
		}
		v.prevCapturePhoto = NewPhoto(Data(pngBytes))
		v.captureLabel.Configure(Image(v.prevCapturePhoto))
	}
	if v.detectionLabel != nil {
		if v.prevDetectionPhoto != nil {
			v.prevDetectionPhoto.Delete()
		}
		v.prevDetectionPhoto = NewPhoto(Data(pngBytes))
		v.detectionLabel.Configure(Image(v.prevDetectionPhoto))
	}
}

// setTargetSize updates desired scaling dimensions used by UpdateCapture.
func (v *capturePreview) setTargetSize(w, h int) {
	if v == nil {
		return
	}
	if w < 50 {
		w = 50
	}
	if h < 50 {
		h = 50
	}
	v.targetW, v.targetH = w, h
}
