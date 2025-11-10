package capture

import (
	"image"
	"sync"
)

// Lightweight reusable frame pool to reduce long-lived heap churn caused by
// repeated allocation of large RGBA backing slices. This does NOT eliminate
// the allocations performed by the underlying screenshot library (it still
// returns a freshly allocated *image.RGBA); we copy those pixels into a pooled
// buffer. For large resolutions this saves the persistent retention of many
// distinct backing slices when consumers process frames slowly, but to fully
// remove per-frame allocations an in-place OS capture (e.g. GDI BitBlt into a
// caller-provided buffer on Windows) would be required.
//
// Usage: acquireFrame(rect) returns a *image.RGBA whose Pix slice capacity is
// at least rect area * 4. After consumers finish using the frame they should
// call recycleFrame(frame) to allow its reuse. If consumers never recycle, the
// behavior degrades gracefully to the previous allocation pattern.

var framePool sync.Pool // stores *image.RGBA

// acquireFrame returns a reusable RGBA image sized to rect. The returned Pix
// length exactly matches rect area * 4, and Stride is width*4.
func acquireFrame(rect image.Rectangle) *image.RGBA {
	w, h := rect.Dx(), rect.Dy()
	if w <= 0 || h <= 0 {
		return &image.RGBA{Rect: rect}
	}
	needed := w * h * 4
	var img *image.RGBA
	if v := framePool.Get(); v != nil {
		img = v.(*image.RGBA)
	}
	if img == nil || cap(img.Pix) < needed {
		img = &image.RGBA{Pix: make([]byte, needed), Stride: w * 4, Rect: rect}
	} else {
		img.Stride = w * 4
		img.Rect = rect
		img.Pix = img.Pix[:needed]
	}
	return img
}

// recycleFrame returns the frame to the pool for potential reuse. The frame
// must no longer be accessed by the caller after invoking recycleFrame.
// RecycleFrame returns the frame to the pool for potential reuse. The frame
// must no longer be accessed by the caller after invoking RecycleFrame.
func RecycleFrame(img *image.RGBA) {
	if img == nil {
		return
	}
	if img.Pix == nil {
		return
	}
	framePool.Put(img)
}
