package capture

import "image"

// FrameSource provides read-only access to captured frames.
// LatestFrame returns the freshest snapshot while Running reports activity.
type FrameSource interface {
	LatestFrame() FrameSnapshot
	Running() bool
}

// SelectionRectProvider returns the current selection rectangle, if any.
type SelectionRectProvider interface{ SelectionRect() *image.Rectangle }

// ServiceContract exposes basic lifecycle control for capture services.
type ServiceContract interface {
	Start()
	Stop()
	Running() bool
}

// ServiceWithSelection extends ServiceContract with a setter for a selection provider.
type ServiceWithSelection interface {
	ServiceContract
	SetSelectionProvider(func() *image.Rectangle)
}

// Service is the capture service interface used by higher-level components.
// It is an alias for the concrete capture service defined elsewhere.
type Service interface{ CaptureService }
