package model

import (
	"sync/atomic"
)

// CaptureModel tracks whether capture is enabled.
//
// The zero value represents disabled capture and is safe to use. Access is
// concurrency-safe: the enabled flag is stored in an atomic.Bool because UI
// callbacks and presenter ticks may access it from different goroutines.
type CaptureModel struct {
	enabled atomic.Bool
	window  atomic.Pointer[string]
}

// Enabled reports whether capture is enabled.
func (m *CaptureModel) Enabled() bool { return m.enabled.Load() }

// Window returns the title of the selected target window, or "".
func (m *CaptureModel) Window() string {
	if title := m.window.Load(); title != nil {
		return *title
	}
	return ""
}

// SetWindow selects the target window by title.
func (m *CaptureModel) SetWindow(title string) { m.window.Store(&title) }

// SetEnabled sets the enabled flag.
func (m *CaptureModel) SetEnabled(b bool) { m.enabled.Store(b) }
