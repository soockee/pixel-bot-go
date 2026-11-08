package app

import (
	"bytes"
	"errors"
	"image"
	"image/draw"
	"image/png"
	"os"
)

func readSchedulePNG(path string) (*image.RGBA, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	cfg, err := png.DecodeConfig(file)
	if err != nil {
		return nil, err
	}
	if cfg.Width < 8 || cfg.Height < 8 || cfg.Width > 512 || cfg.Height > 512 {
		return nil, errors.New("use a PNG crop between 8 and 512 pixels per side")
	}
	if _, err := file.Seek(0, 0); err != nil {
		return nil, err
	}
	img, err := png.Decode(file)
	if err != nil {
		return nil, err
	}
	patch := image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))
	draw.Draw(patch, patch.Bounds(), img, img.Bounds().Min, draw.Src)
	distinct := false
	for i := 0; i < len(patch.Pix); i += 4 {
		if patch.Pix[i+3] != 255 {
			return nil, errors.New("use an opaque PNG screenshot crop")
		}
		distinct = distinct || !bytes.Equal(patch.Pix[i:i+4], patch.Pix[:4])
	}
	if !distinct {
		return nil, errors.New("crop a recognizable control with visible detail")
	}
	return patch, nil
}

// containsScreenPatch accepts an exact match at any pixel in the client.
// Search by the first row before comparing the rest of a candidate.
func containsScreenPatch(frame, patch *image.RGBA) bool {
	b, p := frame.Bounds(), patch.Bounds()
	w, h := p.Dx(), p.Dy()
	if w == 0 || h == 0 || w > b.Dx() || h > b.Dy() {
		return false
	}
	first := patch.Pix[:w*4]
	for y := b.Min.Y; y <= b.Max.Y-h; y++ {
		offset := frame.PixOffset(b.Min.X, y)
		row := frame.Pix[offset : offset+b.Dx()*4]
		for x := 0; x+len(first) <= len(row); {
			found := bytes.Index(row[x:], first)
			if found < 0 {
				break
			}
			x += found
			if x%4 == 0 {
				matches := true
				for dy := 1; dy < h; dy++ {
					start := offset + x + dy*frame.Stride
					if !bytes.Equal(frame.Pix[start:start+w*4], patch.Pix[dy*patch.Stride:dy*patch.Stride+w*4]) {
						matches = false
						break
					}
				}
				if matches {
					return true
				}
			}
			x++
		}
	}
	return false
}
