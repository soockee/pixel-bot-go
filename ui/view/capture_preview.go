package view

import (
	"image"

	"github.com/soocke/pixel-bot-go/ui/images"

	//lint:ignore ST1001 Dot import is intentional for concise Tk widget DSL builders.
	. "modernc.org/tk9.0"
)

const placeholderW, placeholderH = 400, 225

// CapturePreview shows the captured frame and the region around the bobber.
type CapturePreview struct {
	captureLabel   *LabelWidget
	detectionLabel *LabelWidget
	roiSize        int
	targetW        int
	targetH        int
	capturePhoto   *Img
	detectionPhoto *Img
}

func newCapturePreview(capture, detection *LabelWidget, roiSize int) *CapturePreview {
	v := &CapturePreview{captureLabel: capture, detectionLabel: detection, roiSize: roiSize, targetW: 800, targetH: 450}
	v.Reset()
	return v
}

// show replaces the label's photo, deleting the old one so its pixels are freed.
func show(label *LabelWidget, photo **Img, img image.Image) {
	if *photo != nil {
		(*photo).Delete()
	}
	*photo = NewPhoto(Data(images.EncodePNG(img)))
	label.Configure(Image(*photo))
}

// UpdateCapture shows a frame scaled to the preview size.
func (v *CapturePreview) UpdateCapture(img *image.RGBA) {
	show(v.captureLabel, &v.capturePhoto, images.ScaleToFit(img, v.targetW, v.targetH))
}

// UpdateDetection shows the region around the bobber.
func (v *CapturePreview) UpdateDetection(img *image.RGBA) {
	show(v.detectionLabel, &v.detectionPhoto, img)
}

// Reset shows empty placeholders.
func (v *CapturePreview) Reset() {
	show(v.captureLabel, &v.capturePhoto, image.NewRGBA(image.Rect(0, 0, placeholderW, placeholderH)))
	show(v.detectionLabel, &v.detectionPhoto, image.NewRGBA(image.Rect(0, 0, v.roiSize, v.roiSize)))
}

// setTargetSize updates desired scaling dimensions used by UpdateCapture.
func (v *CapturePreview) setTargetSize(w, h int) {
	v.targetW, v.targetH = max(w, 50), max(h, 50)
}
