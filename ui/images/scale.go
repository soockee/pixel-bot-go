package images

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// EncodePNG encodes an image to PNG bytes. Errors are ignored and may return an empty slice.
func EncodePNG(img image.Image) []byte {
	if img == nil {
		return nil
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// ScaleToFit performs a nearest-neighbour scale so that the returned image fits within
// maxW x maxH preserving aspect ratio. If the source already fits, the original is returned.
func ScaleToFit(src image.Image, maxW, maxH int) image.Image {
	if src == nil {
		return nil
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxW && h <= maxH {
		return src
	}
	if maxW < 1 {
		maxW = 1
	}
	if maxH < 1 {
		maxH = 1
	}
	ratioW := float64(maxW) / float64(w)
	ratioH := float64(maxH) / float64(h)
	ratio := ratioW
	if ratioH < ratio {
		ratio = ratioH
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
