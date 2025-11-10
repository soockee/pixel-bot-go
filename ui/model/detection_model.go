package model

import (
	"image"
)

// DetectionModel holds the current global ROI rectangle.
// The zero value (empty rectangle) means no active ROI.
// No synchronization is required because updates happen on the UI thread.
type DetectionModel struct {
	roi image.Rectangle
}

// NewDetectionModel returns an initialized DetectionModel.
func NewDetectionModel() *DetectionModel { return &DetectionModel{} }

// SetROI sets the global ROI. Passing an empty or non-positive rectangle clears the ROI.
func (m *DetectionModel) SetROI(r image.Rectangle) {
	if m == nil {
		return
	}
	if r.Empty() || r.Dx() <= 0 || r.Dy() <= 0 {
		m.roi = image.Rectangle{}
		return
	}
	m.roi = r
}

// ROI returns the current global ROI (may be empty).
func (m *DetectionModel) ROI() image.Rectangle {
	if m == nil {
		return image.Rectangle{}
	}
	return m.roi
}
