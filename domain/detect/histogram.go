package detect

import (
	"image"
	"log/slog"
	"time"
)

// HistogramBiteDetector fires when the colours in the watched box stop
// matching the colours it learned while the bobber floated calmly. It compares
// colour histograms, so it ignores where in the box the bobber bobs and only
// reacts when the splash adds colours the calm water did not have.
// Not safe for concurrent use; call Feed from a single goroutine.
type HistogramBiteDetector struct {
	logger   *slog.Logger
	start    time.Time
	recent   [][histogramBins]uint32 // the last histogramBaseline frames of the warm-up
	baseline []float64
	fired    bool
}

const (
	histogramBins      = 512             // 3 bits per channel
	histogramWarmUp    = 3 * time.Second // landing ripples fade within this time after the lock
	histogramBaseline  = 10              // frames averaged into the baseline, the last of the warm-up
	histogramThreshold = 0.75            // a bite drops the similarity to the baseline below this
)

// NewHistogramBiteDetector returns a detector. A nil logger discards logs.
func NewHistogramBiteDetector(logger *slog.Logger) *HistogramBiteDetector {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &HistogramBiteDetector{logger: logger}
}

// Reset forgets the baseline, ready for the next cast.
func (h *HistogramBiteDetector) Reset() { *h = HistogramBiteDetector{logger: h.logger} }

// Feed takes one frame of the watched box sampled at t and reports a bite once.
func (h *HistogramBiteDetector) Feed(roi *image.RGBA, t time.Time) bool {
	if roi == nil || h.fired || roi.Bounds().Empty() {
		return false
	}
	hist := histogram(roi)
	if h.start.IsZero() {
		h.start = t
	}
	if t.Sub(h.start) < histogramWarmUp {
		h.recent = append(h.recent, hist)
		if len(h.recent) > histogramBaseline {
			h.recent = h.recent[1:]
		}
		return false
	}
	if h.baseline == nil {
		h.baseline = make([]float64, histogramBins)
		for _, r := range h.recent {
			for i, n := range r {
				h.baseline[i] += float64(n) / float64(len(h.recent))
			}
		}
	}
	similarity := intersection(hist, h.baseline)
	h.fired = similarity < histogramThreshold
	if h.fired {
		h.logger.Info("bite detected", "detector", "histogram", "similarity", similarity)
	}
	return h.fired
}

var _ BiteDetector = (*HistogramBiteDetector)(nil)

// histogram counts the pixels of roi in 512 colour bins.
func histogram(roi *image.RGBA) [histogramBins]uint32 {
	var hist [histogramBins]uint32
	r := roi.Bounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := roi.Pix[roi.PixOffset(r.Min.X, y):]
		for x := range r.Dx() {
			p := row[x*4:]
			hist[int(p[0]>>5)|int(p[1]>>5)<<3|int(p[2]>>5)<<6]++
		}
	}
	return hist
}

// intersection is the share of pixels that fall in bins both histograms fill:
// 1 for the same colours, 0 for none in common.
func intersection(hist [histogramBins]uint32, base []float64) float64 {
	var common, a, b float64
	for i, n := range hist {
		common += min(float64(n), base[i])
		a += float64(n)
		b += base[i]
	}
	if min(a, b) == 0 {
		return 1 // no evidence: an empty box is not a bite
	}
	return common / min(a, b)
}
