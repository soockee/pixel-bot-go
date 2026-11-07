// Package capture grabs frames from a region of the screen.
package capture

import (
	"image"
	"time"
)

// Frame is one captured image. Pixels are read-only once a frame is handed out.
type Frame struct {
	Image      *image.RGBA
	Origin     image.Point // screen position of the image's top-left corner
	CapturedAt time.Time
}

// Source grabs frames from the rectangle Region returns.
type Source struct {
	Region func() *image.Rectangle
}
