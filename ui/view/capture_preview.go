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
	captureLabel   *LabelWidget
	detectionLabel *LabelWidget
	targetW        int
	targetH        int
}

// NewCapturePreview creates the preview labels, grids them and returns the view.
// Layout: capture spans columns 0-3; detection ROI sits at column 4 of the provided row.
func NewCapturePreview(row int) CapturePreview {
	placeholder := image.NewRGBA(image.Rect(0, 0, 200, 120))
	pngBytes := images.EncodePNG(placeholder)
	capture := Label(Image(NewPhoto(Data(pngBytes))), Borderwidth(1), Relief("sunken"))
	detection := Label(Image(NewPhoto(Data(pngBytes))), Borderwidth(1), Relief("sunken"))
	Grid(capture, Row(row), Column(0), Columnspan(4), Sticky("we"), Padx("0.4m"), Pady("0.4m"))
	Grid(detection, Row(row), Column(4), Columnspan(1), Sticky("we"), Padx("0.4m"), Pady("0.4m"))
	return &capturePreview{captureLabel: capture, detectionLabel: detection}
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
	// Determine target size (fallback to legacy constants if unset)
	w, h := v.targetW, v.targetH
	if w <= 0 || h <= 0 {
		w, h = maxPreviewW, maxPreviewH
	}
	// Scale for display only; original frame retained upstream for analysis.
	scaled := images.ScaleToFit(img, w, h)
	v.captureLabel.Configure(Image(NewPhoto(Data(images.EncodePNG(scaled)))))
}

func (v *capturePreview) UpdateDetection(img image.Image) {
	if v.detectionLabel == nil {
		return
	}
	if img == nil {
		return
	}
	v.detectionLabel.Configure(Image(NewPhoto(Data(images.EncodePNG(img)))))
}

func (v *capturePreview) Reset() {
	placeholder := image.NewRGBA(image.Rect(0, 0, 200, 120))
	pngBytes := images.EncodePNG(placeholder)
	if v.captureLabel != nil {
		v.captureLabel.Configure(Image(NewPhoto(Data(pngBytes))))
	}
	if v.detectionLabel != nil {
		v.detectionLabel.Configure(Image(NewPhoto(Data(pngBytes))))
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
