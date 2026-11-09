package detect

import (
	"image"
	"log/slog"
	"slices"
	"time"
)

// MotionBiteDetector fires on the first bite centreMotion sees.
// Not safe for concurrent use; call Feed from a single goroutine.
type MotionBiteDetector struct {
	logger *slog.Logger
	motion centreMotion
	fired  bool
}

// NewMotionBiteDetector returns a detector. A nil logger discards logs.
func NewMotionBiteDetector(logger *slog.Logger) *MotionBiteDetector {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &MotionBiteDetector{logger: logger}
}

// Reset clears the state for a new cast.
func (b *MotionBiteDetector) Reset() { b.motion, b.fired = centreMotion{}, false }

// Feed processes one frame of the watched box sampled at time t and returns
// true once, when a bite is detected.
func (b *MotionBiteDetector) Feed(frame *image.RGBA, t time.Time) bool {
	if b.fired {
		return false
	}
	m, hit := b.motion.observe(frame, t)
	if hit {
		b.fired = true
		b.logger.Info("bite detected", "detector", "motion", "change", m.change, "around", m.around,
			"threshold", m.threshold, "box_w", b.motion.size.X, "box_h", b.motion.size.Y)
	}
	return hit
}

// centreMotion watches the centre of the watched box, where the addon's
// brackets put the bobber. It counts the centre's pixels that changed strongly
// against the frame lagFor earlier, so the frame rate does not matter, and
// reports a hit when that count clearly exceeds the water's own recent level,
// measured from the same box. Counting, not averaging, keeps the small splash
// of a far bobber, a few dozen pixels in a large box, from being diluted. It
// ignores the first LandingSettle, while the bobber lands, and a change as dense
// around the centre as at it, such as a camera turn.
//
// In recorded casts (detecteval -export) the water changes by about 1 grey
// level per 100 ms on average, the landing by 15 to 50 in the first second,
// and a bite rises over 0.3 to 0.6 s until 15 to 60 % of the centre changes,
// two to three times as much as around it.
type centreMotion struct {
	first   time.Time   // the first frame since the box was set
	frames  []lagFrame  // recent centre and ring luminance, oldest first
	history []float64   // the centre's change while nothing happened, newest last
	above   int         // frames in a row over the threshold
	size    image.Point // the box size frames have
}

type lagFrame struct {
	at           time.Time
	centre, ring []float64
}

// LandingSettle is how long after the first frame of a cast centreMotion
// ignores changes: the bobber lands and the marker appears. No bite comes that
// early.
const LandingSettle = 1500 * time.Millisecond

// motionMeasure is what centreMotion saw in a frame: the centre's strongly
// changed pixels, those around it scaled to the centre's area, and the count a
// hit needs.
type motionMeasure struct{ change, around, threshold float64 }

const (
	lagFor       = 100 * time.Millisecond // compare with the frame this long ago
	historyLen   = 90                     // about 3 s of water at 30 fps
	minHistory   = 5                      // frames before the threshold means anything
	strongChange = 20                     // grey levels a pixel must change by to count
	spreadFactor = 6.0                    // a bite is this many spreads (MAD) over the median
	minRise      = 8.0                    // and at least this many pixels over it
	focusRatio   = 1.3                    // and this much denser at the centre than around it
	framesNeeded = 2                      // frames in a row over the threshold
	strongFactor = 3.0                    // or one frame this many times over it
	strongPixels = 30                     // with at least this many pixels changed
)

// observe takes one frame sampled at t and reports whether the centre has
// clearly changed for framesNeeded frames in a row.
func (c *centreMotion) observe(frame *image.RGBA, t time.Time) (motionMeasure, bool) {
	var m motionMeasure
	if frame == nil || frame.Bounds().Empty() {
		return m, false
	}
	if size := frame.Bounds().Size(); size != c.size {
		*c = centreMotion{size: size}
	}
	if c.first.IsZero() {
		c.first = t
	}
	centre, ring := splitLuminance(frame)
	var old *lagFrame
	for i := range c.frames {
		if t.Sub(c.frames[i].at) >= lagFor {
			old = &c.frames[i]
		}
	}
	c.keep(lagFrame{at: t, centre: centre, ring: ring})
	if old == nil {
		return m, false
	}
	m.change = float64(countChanged(centre, old.centre))
	if len(ring) > 0 {
		m.around = float64(countChanged(ring, old.ring)) * float64(len(centre)) / float64(len(ring))
	}
	if t.Sub(c.first) < LandingSettle {
		return m, false // the landing is not the water's normal level either
	}
	if len(c.history) < minHistory {
		c.remember(m.change)
		return m, false
	}
	median, spread := medianSpread(c.history)
	m.threshold = median + max(spreadFactor*spread, minRise)
	if m.change <= m.threshold || m.change <= focusRatio*m.around {
		c.above = 0
		c.remember(m.change)
		return m, false
	}
	c.above++ // a rising bite stays out of the history
	// A sudden, strong change counts at once: at 10 fps a splash may show in
	// one lagged frame only.
	return m, c.above >= framesNeeded || m.change >= strongFactor*m.threshold && m.change >= strongPixels
}

// keep stores f, dropping frames no longer needed for the lag.
func (c *centreMotion) keep(f lagFrame) {
	drop := 0
	for drop < len(c.frames)-1 && f.at.Sub(c.frames[drop+1].at) >= lagFor {
		drop++
	}
	c.frames = append(c.frames[drop:], f)
}

func (c *centreMotion) remember(change float64) {
	if len(c.history) == historyLen {
		c.history = c.history[1:]
	}
	c.history = append(c.history, change)
}

// splitLuminance returns the grey levels of the centre half of frame (the
// middle half in each direction) and of the ring around it.
func splitLuminance(frame *image.RGBA) (centre, ring []float64) {
	b := frame.Bounds()
	w, h := b.Dx(), b.Dy()
	centre = make([]float64, 0, w*h/4+1)
	ring = make([]float64, 0, w*h)
	for y := range h {
		row := frame.Pix[y*frame.Stride : y*frame.Stride+w*4]
		inY := y >= h/4 && y < h-h/4
		for x := range w {
			i := x * 4
			v := float64(77*uint32(row[i])+150*uint32(row[i+1])+29*uint32(row[i+2])) / 256
			if inY && x >= w/4 && x < w-w/4 {
				centre = append(centre, v)
			} else {
				ring = append(ring, v)
			}
		}
	}
	return centre, ring
}

// countChanged counts the positions where two equally long slices differ by
// more than strongChange, 0 when their lengths differ.
func countChanged(a, b []float64) int {
	if len(a) != len(b) {
		return 0
	}
	n := 0
	for i, v := range a {
		if d := v - b[i]; d > strongChange || d < -strongChange {
			n++
		}
	}
	return n
}

// medianSpread returns the median of v and its median absolute deviation.
func medianSpread(v []float64) (median, spread float64) {
	s := slices.Clone(v)
	slices.Sort(s)
	median = s[len(s)/2]
	for i, x := range s {
		if x < median {
			s[i] = median - x
		} else {
			s[i] = x - median
		}
	}
	slices.Sort(s)
	return median, s[len(s)/2]
}
