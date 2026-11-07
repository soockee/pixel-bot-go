package detect

import (
	"image"

	"github.com/soocke/pixel-bot-go/config"
)

// NCCLocator finds a template with multi-scale normalized cross-correlation.
// Its results depend only on the frame, so runs are repeatable. It reuses
// buffers between calls, so call Locate from one goroutine.
type NCCLocator struct {
	templates []scaledTemplate
	size      image.Point // template size at scale 1
	cfg       config.Config
	pre       grayPrecomp
}

// NewNCCLocator prepares tmpl at each scale of a validated copy of cfg.
func NewNCCLocator(tmpl image.Image, cfg config.Config) *NCCLocator {
	_ = cfg.Validate()
	return &NCCLocator{
		templates: prepareTemplates(tmpl, scaleRange(cfg.MinScale, cfg.MaxScale, cfg.ScaleStep)),
		size:      tmpl.Bounds().Size(),
		cfg:       cfg,
	}
}

// Locate searches frame for the template.
func (l *NCCLocator) Locate(frame *image.RGBA) (Match, bool) {
	l.pre.load(frame)
	res := matchScales(&l.pre, l.templates, NCCOptions{Threshold: l.cfg.Threshold, Stride: l.cfg.Stride, Refine: l.cfg.Refine})
	x := res.X + int(float64(l.size.X)*res.Scale)/2
	y := res.Y + int(float64(l.size.Y)*res.Scale)/2
	half := l.cfg.ROISizePx / 2
	return Match{
		X:     x,
		Y:     y,
		Box:   image.Rect(x-half, y-half, x+half, y+half),
		Score: res.Score,
		Scale: res.Scale,
	}, res.Found
}
