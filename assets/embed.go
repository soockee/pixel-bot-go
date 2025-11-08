package assets

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/png"
)

// FishingTargetPNG contains the raw PNG bytes of the fishing target image.
//
//go:embed fishing_target.png
var FishingTargetPNG []byte

// FishingTargetImage decodes the embedded PNG into an image.Image.
func FishingTargetImage() (image.Image, error) {
	if len(FishingTargetPNG) == 0 {
		return nil, fmt.Errorf("embedded fishing_target.png is empty")
	}
	img, err := png.Decode(bytes.NewReader(FishingTargetPNG))
	if err != nil {
		return nil, err
	}
	return img, nil
}
