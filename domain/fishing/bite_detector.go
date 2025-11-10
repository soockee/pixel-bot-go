package fishing

import (
	"image"
	"log/slog"
	"math"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

// BiteDetector detects bites from ROI frames.
// Not safe for concurrent use; call FeedFrame from a single goroutine.
type BiteDetector struct {
	cfg                                                                  *config.Config
	logger                                                               *slog.Logger
	monitoringStarted                                                    time.Time
	prev                                                                 []byte
	ema                                                                  []byte
	cur                                                                  []byte
	w, h                                                                 int
	window                                                               []float64
	wIdx                                                                 int
	wCount                                                               int
	frameCnt                                                             int
	triggered                                                            bool
	lastSpike                                                            time.Time
	lastFrameTS                                                          time.Time
	prevCandidate                                                        bool
	candidateFrames                                                      int
	statsFrozen                                                          bool
	framesCandidateStarted                                               int
	framesCandidateAborted                                               int
	maxConsecutiveCandidate                                              int
	minDT, maxDT                                                         float64
	minRatioChanged, maxRatioChanged                                     float64
	minDiffBaseMean, maxDiffBaseMean                                     float64
	lastDT, lastRatioChanged, lastDiffBaseMean                           float64
	lastCandidateSpike, lastCandidateBaseJump, lastCandidateBigImmediate bool
}

const (
	windowSize          = 20
	minFramesForStats   = 5
	pixelDiffThreshold  = 10
	ratioThresholdSpike = 0.18
	ratioThresholdBase  = 0.12
	baselineDiffThresh  = 14
	stdDevMultiplier    = 2.0
	bigImmediateRatio   = 0.20
	bigImmediateDiff    = 12
	emaAlpha            = 0.03
	frameDebounceNeeded = 1
)

// NewBiteDetector returns a configured BiteDetector. If cfg is nil the
// default configuration is used.
func NewBiteDetector(cfg *config.Config, logger *slog.Logger) *BiteDetector {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	return &BiteDetector{cfg: cfg, logger: logger, window: make([]float64, windowSize)}
}

// Reset clears internal state and statistics.
func (b *BiteDetector) Reset() {
	b.monitoringStarted = time.Now()
	b.prev, b.ema, b.cur = nil, nil, nil
	b.w, b.h = 0, 0
	b.wIdx, b.wCount, b.frameCnt = 0, 0, 0
	b.triggered = false
	b.lastSpike = time.Time{}
	b.lastFrameTS = time.Time{}
	b.prevCandidate = false
	b.candidateFrames = 0
	b.statsFrozen = false
	b.framesCandidateStarted = 0
	b.framesCandidateAborted = 0
	b.maxConsecutiveCandidate = 0
	b.minDT, b.maxDT = 0, 0
	b.minRatioChanged, b.maxRatioChanged = 0, 0
	b.minDiffBaseMean, b.maxDiffBaseMean = 0, 0
	b.lastDT, b.lastRatioChanged, b.lastDiffBaseMean = 0, 0, 0
	b.lastCandidateSpike, b.lastCandidateBaseJump, b.lastCandidateBigImmediate = false, false, false
	for i := range b.window {
		b.window[i] = 0
	}
}

// FeedFrame processes one ROI frame sampled at time t and returns true when
// a bite is detected. Call from a single goroutine.
func (b *BiteDetector) FeedFrame(frame *image.RGBA, t time.Time) bool {
	if frame == nil || b.triggered {
		return false
	}
	fb := frame.Bounds()
	w, h := fb.Dx(), fb.Dy()
	n := w * h
	if w <= 0 || h <= 0 {
		return false
	}
	if b.prev == nil || w != b.w || h != b.h {
		b.prev = make([]byte, n)
		b.ema = make([]byte, n)
		b.cur = make([]byte, n)
		b.w, b.h = w, h
	}
	pix := frame.Pix
	stride := frame.Stride
	idx := 0
	for y := 0; y < h; y++ {
		row := pix[y*stride : y*stride+w*4]
		for x := 0; x < w; x++ {
			i := x * 4
			r, g, bb := row[i], row[i+1], row[i+2]
			b.cur[idx] = byte((77*uint32(r) + 150*uint32(g) + 29*uint32(bb)) >> 8)
			idx++
		}
	}
	if b.frameCnt == 0 {
		copy(b.prev, b.cur)
		copy(b.ema, b.cur)
		b.frameCnt++
		b.lastFrameTS = t
		return false
	}
	var sumPrev, sumBase int
	changedPixels := 0
	for i := 0; i < n; i++ {
		diffPrev := int(b.cur[i]) - int(b.prev[i])
		if diffPrev < 0 {
			diffPrev = -diffPrev
		}
		sumPrev += diffPrev
		if diffPrev > pixelDiffThreshold {
			changedPixels++
		}
		diffBase := int(b.cur[i]) - int(b.ema[i])
		if diffBase < 0 {
			diffBase = -diffBase
		}
		sumBase += diffBase
	}
	dt := float64(sumPrev) / float64(n)
	ratioChanged := float64(changedPixels) / float64(n)
	diffBaseMean := float64(sumBase) / float64(n)
	var mean, m2 float64
	for i := 0; i < b.wCount; i++ {
		x := b.window[i]
		if i == 0 {
			mean = x
			continue
		}
		delta := x - mean
		mean += delta / float64(i+1)
		m2 += delta * (x - mean)
	}
	std := 0.0
	if b.wCount > 1 {
		std = (m2 / float64(b.wCount-1))
		if std > 0 {
			std = math.Sqrt(std)
		}
	}
	spike := (b.wCount >= minFramesForStats) && (dt > mean+stdDevMultiplier*std) && (ratioChanged > ratioThresholdSpike)
	baseJump := (diffBaseMean > baselineDiffThresh) && (ratioChanged > ratioThresholdBase)
	bigImmediate := (b.wCount < minFramesForStats) && (ratioChanged > bigImmediateRatio) && (dt > bigImmediateDiff)
	candidate := spike || baseJump || bigImmediate
	if b.frameCnt > 0 {
		if b.minDT == 0 && b.maxDT == 0 {
			b.minDT, b.maxDT = dt, dt
			b.minRatioChanged, b.maxRatioChanged = ratioChanged, ratioChanged
			b.minDiffBaseMean, b.maxDiffBaseMean = diffBaseMean, diffBaseMean
		} else {
			if dt < b.minDT {
				b.minDT = dt
			} else if dt > b.maxDT {
				b.maxDT = dt
			}
			if ratioChanged < b.minRatioChanged {
				b.minRatioChanged = ratioChanged
			} else if ratioChanged > b.maxRatioChanged {
				b.maxRatioChanged = ratioChanged
			}
			if diffBaseMean < b.minDiffBaseMean {
				b.minDiffBaseMean = diffBaseMean
			} else if diffBaseMean > b.maxDiffBaseMean {
				b.maxDiffBaseMean = diffBaseMean
			}
		}
		b.lastDT = dt
		b.lastRatioChanged = ratioChanged
		b.lastDiffBaseMean = diffBaseMean
	}
	if candidate {
		b.candidateFrames++
		if !b.prevCandidate {
			b.statsFrozen = true
		}
		if b.candidateFrames > b.maxConsecutiveCandidate {
			b.maxConsecutiveCandidate = b.candidateFrames
		}
		if b.candidateFrames >= frameDebounceNeeded || (bigImmediate && b.candidateFrames == 1) {
			b.triggered = true
			if b.logger != nil {
				b.logger.Info("bite detected", "dt", dt, "meanDt", mean, "stdDt", std, "changedRatio", ratioChanged, "diffBaseMean", diffBaseMean, "framesInCandidate", b.candidateFrames)
			}
			return true
		}
	} else {
		b.candidateFrames = 0
		b.statsFrozen = false
	}
	b.prevCandidate = candidate
	if !b.statsFrozen {
		b.window[b.wIdx] = dt
		b.wIdx = (b.wIdx + 1) % windowSize
		if b.wCount < windowSize {
			b.wCount++
		}
	}
	if !b.triggered {
		for i := 0; i < n; i++ {
			v := int(b.ema[i]) + int(float64(int(b.cur[i])-int(b.ema[i]))*emaAlpha)
			if v < 0 {
				v = 0
			} else if v > 255 {
				v = 255
			}
			b.ema[i] = byte(v)
		}
	}
	copy(b.prev, b.cur)
	b.frameCnt++
	b.lastFrameTS = t
	return false
}

// FeedFrame processes a single ROI frame sampled at t and returns true when
// a bite is detected. FeedFrame is not concurrency-safe and must be called
// from a single goroutine.

func (b *BiteDetector) TargetLostHeuristic() bool {
	if b.cfg == nil || b.cfg.MaxCastDurationSeconds <= 0 {
		return false
	}
	if b.monitoringStarted.IsZero() {
		return false
	}
	limit := time.Duration(b.cfg.MaxCastDurationSeconds) * time.Second
	if time.Since(b.monitoringStarted) >= limit {
		if b.logger != nil {
			b.logger.Info("monitoring timeout elapsed; target considered lost", "limitSec", b.cfg.MaxCastDurationSeconds)
		}
		return true
	}
	return false
}

// compile-time check that BiteDetector implements BiteDetectorContract.
var _ BiteDetectorContract = (*BiteDetector)(nil)
