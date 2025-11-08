package capture

import (
	"image"
	"runtime"
	"sync"
	"sync/atomic"

	"golang.org/x/image/draw"
)

// ScaleSpec defines one scale factor attempt.
type ScaleSpec struct {
	Factor float64 // e.g. 0.8, 1.0, 1.2
}

// MultiScaleOptions configures multi-scale template matching.
type MultiScaleOptions struct {
	Scales      []ScaleSpec // List of scale factors to try; if empty defaults around 1.0
	NCC         NCCOptions  // NCC options reused per scale
	StopOnScore float64     // Early stop if score >= StopOnScore (>0); 0 disables
	// Adaptive scale generation (used if Scales is empty): generate factors from MinScale to MaxScale inclusive.
	MinScale  float64 // e.g. 0.60
	MaxScale  float64 // e.g. 1.40
	ScaleStep float64 // e.g. 0.05 (must be >0)
}

// MultiScaleResult holds the best match among scales.
type MultiScaleResult struct {
	X, Y  int
	Score float64
	Scale float64
	Found bool
}

// MultiScaleMatch resizes the template to each scale and runs NCC, returning best result.
// This mitigates dimensional differences when the target object appears larger/smaller.
// Assumes uniform scaling without rotation.
func MultiScaleMatch(frame *image.RGBA, tmpl image.Image, opts MultiScaleOptions) MultiScaleResult {
	// Delegate to parallel version for scalability; keeps existing API.
	return MultiScaleMatchParallel(frame, tmpl, opts)
}

// MultiScaleMatchParallel performs the same operation as MultiScaleMatch but distributes
// scale evaluations across goroutines for better performance when many scales are tested.
// It respects StopOnScore via an atomic early-stop flag.
func MultiScaleMatchParallel(frame *image.RGBA, tmpl image.Image, opts MultiScaleOptions) MultiScaleResult {
	if frame == nil || tmpl == nil {
		return MultiScaleResult{}
	}
	if len(opts.Scales) == 0 {
		// Generate adaptive list if parameters look sane; else fallback to legacy defaults.
		if opts.MinScale > 0 && opts.MaxScale > 0 && opts.ScaleStep > 0 && opts.MaxScale >= opts.MinScale {
			// Cap number of steps to prevent runaway.
			maxSteps := 1 + int((opts.MaxScale-opts.MinScale)/opts.ScaleStep+0.5)
			if maxSteps > 200 { // arbitrary safety cap
				maxSteps = 200
			}
			scales := make([]ScaleSpec, 0, maxSteps)
			for s := opts.MinScale; s <= opts.MaxScale+1e-9 && len(scales) < maxSteps; s += opts.ScaleStep {
				scales = append(scales, ScaleSpec{Factor: s})
			}
			opts.Scales = scales
		} else {
			// Legacy default set: moderate range around 1.0
			opts.Scales = []ScaleSpec{{0.75}, {0.85}, {0.95}, {1.0}, {1.05}, {1.15}, {1.25}}
		}
	}

	var earlyStop int32 // 0 = continue, 1 = stop requested
	results := make(chan MultiScaleResult, len(opts.Scales))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU()) // bound concurrency

	for _, s := range opts.Scales {
		scale := s.Factor
		if scale <= 0 {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(factor float64) {
			defer wg.Done()
			defer func() { <-sem }()
			if atomic.LoadInt32(&earlyStop) == 1 {
				return
			}
			origB := tmpl.Bounds()
			w := int(float64(origB.Dx()) * factor)
			h := int(float64(origB.Dy()) * factor)
			if w < 2 || h < 2 {
				return
			}
			var res NCCResult
			if factor == 1.0 {
				res = MatchTemplateNCC(frame, tmpl, opts.NCC)
			} else {
				scaled := image.NewRGBA(image.Rect(0, 0, w, h))
				draw.CatmullRom.Scale(scaled, scaled.Bounds(), tmpl, origB, draw.Over, nil)
				res = MatchTemplateNCC(frame, scaled, opts.NCC)
			}
			msr := MultiScaleResult{X: res.X, Y: res.Y, Score: res.Score, Scale: factor, Found: res.Found}
			if opts.StopOnScore > 0 && res.Score >= opts.StopOnScore {
				if atomic.CompareAndSwapInt32(&earlyStop, 0, 1) {
					results <- msr
				}
				return
			}
			results <- msr
		}(scale)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	best := MultiScaleResult{Score: -1}
	for r := range results {
		if r.Score > best.Score {
			best = r
		}
		// If early stop triggered and we already processed that result, we can break.
		if atomic.LoadInt32(&earlyStop) == 1 && r.Score >= opts.StopOnScore && opts.StopOnScore > 0 {
			break
		}
	}
	return best
}
