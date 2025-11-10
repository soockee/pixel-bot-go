package model

import (
	"image"
)

// DetectionModel holds the current global ROI rectangle. Zero value means no active ROI and is usable.
// No synchronization needed: updates occur on the UI thread tick.
type DetectionModel struct {
	roi image.Rectangle
}

func NewDetectionModel() *DetectionModel { return &DetectionModel{} }

// SetROI sets the rectangle (global coordinates). Use an empty rect to clear.
func (m *DetectionModel) SetROI(r image.Rectangle) {
	if m == nil {
		return
	}
	// classify rectangle
	if r.Empty() || r.Dx() <= 0 || r.Dy() <= 0 {
		// treat as clear; report invalid if dimensions non-positive
		m.roi = image.Rectangle{}
		return
	}
	m.roi = r
}

// ROI returns the current rectangle (may be empty).
func (m *DetectionModel) ROI() image.Rectangle {
	if m == nil {
		return image.Rectangle{}
	}
	return m.roi
}
