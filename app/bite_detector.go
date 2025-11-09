package app

import (
	"image"
	"log/slog"
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
}

// Internal detection tuning constants (less sensitive; adjust empirically).
const (
	windowSize          = 20   // frames (~1s at ~20fps)
	minFramesForStats   = 7    // require more frames before using z-score spike logic
	pixelDiffThreshold  = 16   // require larger per-pixel change to count toward changed pixel ratio
	ratioThresholdSpike = 0.28 // higher changed pixel ratio needed for spike condition
	ratioThresholdBase  = 0.18 // higher changed pixel ratio for baseline departure
	baselineDiffThresh  = 22   // higher absolute baseline departure threshold
	stdDevMultiplier    = 2.8  // higher z-score threshold for spike persistence
	bigImmediateRatio   = 0.45 // require even larger early large-change shortcut
	bigImmediateDiff    = 24   // paired with bigImmediateRatio (higher)
	emaAlpha            = 0.03 // slightly slower baseline adaptation
	frameDebounceNeeded = 3    // require more consecutive candidate frames to trigger
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
			std = sqrt(std)
		}
	}

	// Spike conditions (evaluated before window update; mean/std from previous frames only).
	spike := (b.wCount >= minFramesForStats) && (dt > mean+stdDevMultiplier*std) && (ratioChanged > ratioThresholdSpike)
	baseJump := (diffBaseMean > baselineDiffThresh) && (ratioChanged > ratioThresholdBase)
	bigImmediate := (b.wCount < minFramesForStats) && (ratioChanged > bigImmediateRatio) && (dt > bigImmediateDiff)
	candidate := spike || baseJump || bigImmediate

	// Debug logging for state transitions and periodic metrics
	if b.cfg != nil && b.cfg.Debug && b.logger != nil {
		// Transition logs
		if candidate && !b.prevCandidate {
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
		} else if !candidate && b.prevCandidate {
			b.logger.Debug("bite candidate cleared",
				"dt", dt,
				"meanDt", mean,
				"stdDt", std,
				"changedRatio", ratioChanged,
				"diffBaseMean", diffBaseMean,
				"frames", b.frameCnt,
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
			)
		}
	}

	if candidate {
		b.candidateFrames++
		if !b.prevCandidate { // first frame of candidate sequence
			b.statsFrozen = true // freeze mean/std (window not updated) during candidate run
		}
		if b.candidateFrames >= frameDebounceNeeded {
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

// sqrt is a tiny helper (avoid importing math for single use); uses Newton iteration.
func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Initial guess.
	g := x
	for i := 0; i < 6; i++ { // sufficient for our small precision needs
		g = 0.5 * (g + x/g)
	}
	return g
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
