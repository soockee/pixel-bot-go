package detect

import (
	"image"
	"log/slog"
	"math"
	"time"
)

// SplashBiteDetector fires when the bobber's splash brightens the centre of the
// watched box. Spell effects of other players brighten the whole box, so at
// least half of the brightened pixels must lie in the centre.
// Not safe for concurrent use; call Feed from a single goroutine.
type SplashBiteDetector struct {
	logger *slog.Logger
	scene  brightening
	fired  bool
}

const (
	splashSettle    = 3 * time.Second        // landing ripples fade within this time after the lock
	splashRise      = 35                     // luminance above the baseline that counts as brightened
	splashLag       = 500 * time.Millisecond // the baseline takes on lasting changes, such as the landed bobber or a parked cursor
	splashMinCentre = 0.04                   // share of the centre disc that must brighten
)

// NewSplashBiteDetector returns a detector. A nil logger discards logs.
func NewSplashBiteDetector(logger *slog.Logger) *SplashBiteDetector {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &SplashBiteDetector{logger: logger}
}

// Reset forgets the scene, ready for the next cast.
func (s *SplashBiteDetector) Reset() { *s = SplashBiteDetector{logger: s.logger} }

// Feed takes one frame of the watched box sampled at t and reports a bite once.
func (s *SplashBiteDetector) Feed(roi *image.RGBA, t time.Time) bool {
	if roi == nil || s.fired {
		return false
	}
	b, ok := s.scene.measure(roi, t)
	if !ok {
		return false
	}
	s.fired = b.centreSplash() && 2*b.centre >= b.all
	if s.fired {
		s.logger.Info("bite detected", "detector", "splash", "centre", b.centre, "brightened", b.all, "disc", b.disc)
	}
	return s.fired
}

var _ BiteDetector = (*SplashBiteDetector)(nil)

// brightening compares each frame of the watched box with a baseline that
// follows the scene with a lag, so an abrupt splash stands out.
type brightening struct {
	base        []float32
	w, h        int
	start, last time.Time
}

// brightened counts pixels that rose by splashRise, in the whole box and in the
// centre disc where the bobber sits.
type brightened struct{ all, centre, disc int }

func (b brightened) centreSplash() bool {
	return b.centre > 0 && float64(b.centre) >= splashMinCentre*float64(b.disc)
}

// measure updates the baseline with roi and reports what brightened. It reports
// false until the scene has settled after the first frame.
func (s *brightening) measure(roi *image.RGBA, t time.Time) (brightened, bool) {
	r := roi.Bounds()
	w, h := r.Dx(), r.Dy()
	first := s.base == nil || w != s.w || h != s.h
	if first {
		s.base, s.w, s.h, s.start = make([]float32, w*h), w, h, t
	}
	follow := float32(1 - math.Exp(-float64(t.Sub(s.last))/float64(splashLag)))
	s.last = t
	cx, cy, radius := w/2, h/2, min(w, h)/4
	var b brightened
	for y := range h {
		row := roi.Pix[y*roi.Stride:]
		for x := range w {
			p := row[x*4:]
			l := float32(77*uint32(p[0])+150*uint32(p[1])+29*uint32(p[2])) / 256
			i := y*w + x
			if first {
				s.base[i] = l
				continue
			}
			inDisc := (x-cx)*(x-cx)+(y-cy)*(y-cy) <= radius*radius
			if inDisc {
				b.disc++
			}
			if l-s.base[i] > splashRise {
				b.all++
				if inDisc {
					b.centre++
				}
			}
			s.base[i] += (l - s.base[i]) * follow
		}
	}
	return b, !first && t.Sub(s.start) >= splashSettle
}
