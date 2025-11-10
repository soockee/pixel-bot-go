package model

import (
	"sync/atomic"
)

// CaptureModel tracks whether capture is enabled.
//
// The zero value represents disabled capture and is safe to use. Access is
// concurrency-safe: the enabled flag is stored in an atomic.Bool because UI
// callbacks and presenter ticks may access it from different goroutines.
type CaptureModel struct{ enabled atomic.Bool }

// Enabled reports whether capture is enabled.
func (m *CaptureModel) Enabled() bool {
	if m == nil {
		return false
	}
	return m.enabled.Load()
}

// SetEnabled sets the enabled flag.
func (m *CaptureModel) SetEnabled(b bool) {
	if m == nil {
		return
	}
	prev := m.enabled.Load()
	if prev == b { // no change
		return
	}
	m.enabled.Store(b)
}
