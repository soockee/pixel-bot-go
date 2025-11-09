package app

import (
	"image"
	"log/slog"
	"math"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

// BiteDetector implements visual bite detection during Monitoring state.
// It ingests a short sequence (~1s) of ROI frames and fires once on a
// significant change spike. Not concurrency-safe; call from single goroutine.
type BiteDetector struct {
	cfg    *config.Config
	logger *slog.Logger

	// Timing
	monitoringStarted time.Time

	// Frame buffers / metrics (grayscale luminance per pixel)
	prev []byte // previous frame luminance
	ema  []byte // slow exponentially weighted baseline
	cur  []byte // scratch for current frame luminance
	w, h int

	// Rolling statistics of per-frame mean absolute diff to previous (d_t)
	window   []float64 // circular buffer
	wIdx     int
	wCount   int
	frameCnt int

	// Trigger state
	triggered       bool
	lastSpike       time.Time // first frame time that matched candidate criteria (for debounce)
	lastFrameTS     time.Time // time of last FeedFrame call
	prevCandidate   bool      // last frame candidate state (for debug transition logs)
	candidateFrames int       // consecutive candidate frames count (frame-based debounce)
	statsFrozen     bool      // once candidate starts, freeze mean/std update until trigger/clear

	// Instrumentation / diagnostics
	framesCandidateStarted                                               int
	framesCandidateAborted                                               int
	maxConsecutiveCandidate                                              int
	minDT, maxDT                                                         float64
	minRatioChanged, maxRatioChanged                                     float64
	minDiffBaseMean, maxDiffBaseMean                                     float64
	lastDT, lastRatioChanged, lastDiffBaseMean                           float64
	lastCandidateSpike, lastCandidateBaseJump, lastCandidateBigImmediate bool
}

// Internal detection tuning constants (tuned for increased sensitivity; still conservative but lower than original).
const (
	windowSize          = 20   // frames (~1s at ~20fps)
	minFramesForStats   = 5    // fewer frames needed before spike logic uses z-score
	pixelDiffThreshold  = 10   // per-pixel diff threshold (captures moderate changes)
	ratioThresholdSpike = 0.18 // changed pixel ratio for spike condition
	ratioThresholdBase  = 0.12 // changed pixel ratio for baseline departure
	baselineDiffThresh  = 14   // absolute baseline departure threshold
	stdDevMultiplier    = 2.0  // z-score threshold
	bigImmediateRatio   = 0.20 // early large-change shortcut ratio (allows 0.25 region changes)
	bigImmediateDiff    = 12   // paired diff threshold for bigImmediateRatio
	emaAlpha            = 0.03 // baseline adaptation speed
	frameDebounceNeeded = 1    // allow single-frame spike detection
)

func NewBiteDetector(cfg *config.Config, logger *slog.Logger) *BiteDetector {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	return &BiteDetector{cfg: cfg, logger: logger, window: make([]float64, windowSize)}
}

// Reset clears internal buffers (call on Monitoring entry).
func (b *BiteDetector) Reset() {
	b.monitoringStarted = time.Now()
	b.prev = nil
	b.ema = nil
	b.cur = nil
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
	// Clear window contents (optional; wCount=0 already prevents use)
	for i := range b.window {
		b.window[i] = 0
	}
}

// FeedFrame ingests an ROI frame (RGBA) at time t. Returns true exactly once
// when a bite is detected (large short-term visual change).
func (b *BiteDetector) FeedFrame(frame *image.RGBA, t time.Time) bool {
	if frame == nil || b.triggered {
		return false
	}
	// Handle dimension changes / first frame initialization.
	fb := frame.Bounds()
	w := fb.Dx()
	h := fb.Dy()
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

	// Convert current frame RGBA pixels to 8-bit luminance.
	pix := frame.Pix
	stride := frame.Stride
	idx := 0
	for y := 0; y < h; y++ {
		row := pix[y*stride : y*stride+w*4]
		for x := 0; x < w; x++ {
			i := x * 4
			r := row[i]
			g := row[i+1]
			bb := row[i+2]
			// Integer approx: (77R + 150G + 29B) >> 8
			b.cur[idx] = byte((77*uint32(r) + 150*uint32(g) + 29*uint32(bb)) >> 8)
			idx++
		}
	}

	if b.frameCnt == 0 { // First frame: initialize baseline & prev
		copy(b.prev, b.cur)
		copy(b.ema, b.cur)
		b.frameCnt++
		b.lastFrameTS = t
		return false
	}

	// Compute metrics vs previous and EMA baseline.
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

	dt := float64(sumPrev) / float64(n) // mean absolute diff to previous frame
	ratioChanged := float64(changedPixels) / float64(n)
	diffBaseMean := float64(sumBase) / float64(n)

	// Compute mean & std over existing window BEFORE adding current dt (leave-one-in next step)
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

	// Spike conditions (evaluated before window update; mean/std from previous frames only).
	spike := (b.wCount >= minFramesForStats) && (dt > mean+stdDevMultiplier*std) && (ratioChanged > ratioThresholdSpike)
	baseJump := (diffBaseMean > baselineDiffThresh) && (ratioChanged > ratioThresholdBase)
	bigImmediate := (b.wCount < minFramesForStats) && (ratioChanged > bigImmediateRatio) && (dt > bigImmediateDiff)
	candidate := spike || baseJump || bigImmediate

	// Update instrumentation metrics (after computing values, before potential trigger)
	if b.frameCnt > 0 { // skip first frame initialization
		if b.minDT == 0 && b.maxDT == 0 {
			// first metrics sample
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

	// Debug logging for state transitions and periodic metrics
	if b.cfg != nil && b.cfg.Debug && b.logger != nil {
		if candidate && !b.prevCandidate { // candidate start
			b.framesCandidateStarted++
			b.lastCandidateSpike = spike
			b.lastCandidateBaseJump = baseJump
			b.lastCandidateBigImmediate = bigImmediate
			b.logger.Debug("bite candidate start",
				"dt", dt,
				"meanDt", mean,
				"stdDt", std,
				"changedRatio", ratioChanged,
				"diffBaseMean", diffBaseMean,
				"frames", b.frameCnt,
				"spike", spike,
				"baseJump", baseJump,
				"bigImmediate", bigImmediate,
			)
		} else if !candidate && b.prevCandidate { // candidate cleared
			b.framesCandidateAborted++
			b.logger.Debug("bite candidate cleared",
				"dt", dt,
				"meanDt", mean,
				"stdDt", std,
				"changedRatio", ratioChanged,
				"diffBaseMean", diffBaseMean,
				"frames", b.frameCnt,
				"wasSpike", b.lastCandidateSpike,
				"wasBaseJump", b.lastCandidateBaseJump,
				"wasBigImmediate", b.lastCandidateBigImmediate,
			)
		} else if b.frameCnt%10 == 0 { // periodic snapshot (every ~0.5s at 20fps)
			b.logger.Debug("bite metrics",
				"dt", dt,
				"meanDt", mean,
				"stdDt", std,
				"changedRatio", ratioChanged,
				"diffBaseMean", diffBaseMean,
				"candidate", candidate,
				"spike", spike,
				"baseJump", baseJump,
				"bigImmediate", bigImmediate,
				"minDT", b.minDT,
				"maxDT", b.maxDT,
				"minRatioChanged", b.minRatioChanged,
				"maxRatioChanged", b.maxRatioChanged,
				"minDiffBaseMean", b.minDiffBaseMean,
				"maxDiffBaseMean", b.maxDiffBaseMean,
				"candidateStarts", b.framesCandidateStarted,
				"candidateAborts", b.framesCandidateAborted,
				"maxConsecutiveCandidate", b.maxConsecutiveCandidate,
			)
		}
	}

	if candidate {
		b.candidateFrames++
		if !b.prevCandidate { // first frame of candidate sequence
			b.statsFrozen = true // freeze mean/std (window not updated) during candidate run
		}
		if b.candidateFrames > b.maxConsecutiveCandidate {
			b.maxConsecutiveCandidate = b.candidateFrames
		}
		// Trigger conditions: either debounce satisfied or bigImmediate single-frame spike
		if b.candidateFrames >= frameDebounceNeeded || (bigImmediate && b.candidateFrames == 1) {
			b.triggered = true
			if b.logger != nil {
				b.logger.Info("bite detected", "dt", dt, "meanDt", mean, "stdDt", std, "changedRatio", ratioChanged, "diffBaseMean", diffBaseMean, "framesInCandidate", b.candidateFrames)
			}
			return true
		}
	} else {
		// Candidate ended or not active; unfreeze stats and reset counters.
		b.candidateFrames = 0
		b.statsFrozen = false
	}
	b.prevCandidate = candidate

	// Only update rolling window if stats not frozen (avoids threshold inflation mid-spike)
	if !b.statsFrozen {
		b.window[b.wIdx] = dt
		b.wIdx = (b.wIdx + 1) % windowSize
		if b.wCount < windowSize {
			b.wCount++
		}
	}

	// Update EMA baseline only if not triggered yet.
	if !b.triggered {
		for i := 0; i < n; i++ {
			// ema = ema + alpha*(cur-ema)
			v := int(b.ema[i]) + int(float64(int(b.cur[i])-int(b.ema[i]))*emaAlpha)
			if v < 0 {
				v = 0
			} else if v > 255 {
				v = 255
			}
			b.ema[i] = byte(v)
		}
	}

	// Prepare for next frame.
	copy(b.prev, b.cur)
	b.frameCnt++
	b.lastFrameTS = t
	return false
}

// TargetLostHeuristic returns true once the monitoring duration exceeds the
// configured MaxCastDurationSeconds. Acts as a hard timeout independent of visual changes.
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
