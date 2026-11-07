package images

import (
	"bytes"
	"image"
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

// ScaleToFit performs a nearest-neighbour scale so the result fits within
// maxW x maxH preserving aspect ratio. Images that already fit are copied at
// their own size.
func ScaleToFit(src *image.RGBA, maxW, maxH int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	maxW, maxH = max(maxW, 1), max(maxH, 1)
	ratio := min(float64(maxW)/float64(w), float64(maxH)/float64(h), 1)
	newW := max(int(float64(w)*ratio+0.5), 1)
	newH := max(int(float64(h)*ratio+0.5), 1)
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	for y := range newH {
		row := src.Pix[src.PixOffset(b.Min.X, b.Min.Y+y*h/newH):]
		out := dst.Pix[y*dst.Stride:]
		for x := range newW {
			copy(out[x*4:x*4+4], row[(x*w/newW)*4:])
		}
	}
	return dst
}
