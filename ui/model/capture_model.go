package model

import (
	"sync/atomic"
)

// CaptureModel tracks whether capture is enabled. The zero value is disabled and usable.
// Concurrency-safe via atomic Bool because UI callbacks and presenter ticks may race.
type CaptureModel struct{ enabled atomic.Bool }

// Enabled reports whether capture is currently enabled.
func (m *CaptureModel) Enabled() bool {
	if m == nil {
		return false
	}
	return m.enabled.Load()
}

// SetEnabled stores the enabled flag.
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
