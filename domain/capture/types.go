package capture

import "image"

// FrameSource exposes frame acquisition for presenters without start/stop control.
type FrameSource interface {
	Frames() <-chan *image.RGBA
	Running() bool
}

// SelectionRectProvider supplies an optional active selection rectangle.
type SelectionRectProvider interface{ SelectionRect() *image.Rectangle }

// ServiceContract narrows lifecycle control methods for capture.
type ServiceContract interface {
	Start()
	Stop()
	Running() bool
}

// ServiceWithSelection augments ServiceContract with selection provider mutation.
// Presenters typically do not need this; composition root uses it to wire selection overlay.
type ServiceWithSelection interface {
	ServiceContract
	SetSelectionProvider(func() *image.Rectangle)
}

// CaptureService is the full public-facing interface for the concrete service.
// It intentionally embeds ServiceContract to clarify lifecycle subset.
// NOTE: The legacy CaptureService interface is defined in capture_service.go.
// To avoid redeclaration, we alias it here to satisfy patterns similar to
// domain/fishing where types.go centralizes contracts.
type Service interface{ CaptureService }
