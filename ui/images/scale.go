package images

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// EncodePNG encodes an image to PNG bytes with no compression using a fresh buffer each call.
// This simpler implementation trades reduced allocations for clarity; the GC will reclaim
// buffers when no longer referenced.
func EncodePNG(img image.Image) []byte {
	if img == nil {
		return nil
	}
	var b bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.NoCompression}
	_ = enc.Encode(&b, img)
	return b.Bytes()
}

// ScaleToFit performs a nearest-neighbour scale so the result fits within maxW x maxH
// preserving aspect ratio. A new *image.RGBA is allocated for every call regardless of
// source dimensions; callers should retain the result if they need reuse.
func ScaleToFit(src image.Image, maxW, maxH int) *image.RGBA {
	if src == nil {
		return nil
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if maxW < 1 {
		maxW = 1
	}
	if maxH < 1 {
		maxH = 1
	}
	// If fits already, still allocate a new RGBA for consistency.
	ratioW := float64(maxW) / float64(w)
	ratioH := float64(maxH) / float64(h)
	ratio := ratioW
	if ratioH < ratio {
		ratio = ratioH
	}
	if w <= maxW && h <= maxH {
		ratio = 1.0
	}
	newW := int(float64(w)*ratio + 0.5)
	newH := int(float64(h)*ratio + 0.5)
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	for y := 0; y < newH; y++ {
		sy := int(float64(y) * float64(h) / float64(newH))
		for x := 0; x < newW; x++ {
			sx := int(float64(x) * float64(w) / float64(newW))
			c := src.At(b.Min.X+sx, b.Min.Y+sy)
			r, g, bl, a := c.RGBA()
			dst.SetRGBA(x, y, color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), uint8(a >> 8)})
		}
	}
	return dst
}
