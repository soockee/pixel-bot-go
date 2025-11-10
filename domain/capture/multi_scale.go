package capture

import (
	"image"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// ScaleSpec is a single template scale factor (e.g. 0.8, 1.0, 1.2).
type ScaleSpec struct {
	Factor float64
}

// MultiScaleOptions configures multi-scale template matching.
// Scales: explicit factors to try. If empty, factors are generated from
// MinScale..MaxScale using ScaleStep. StopOnScore disables when set to 0.
type MultiScaleOptions struct {
	Scales      []ScaleSpec
	NCC         NCCOptions
	StopOnScore float64
	MinScale    float64
	MaxScale    float64
	ScaleStep   float64
}

// MultiScaleResult is the best match found across scales.
type MultiScaleResult struct {
	X, Y            int
	Score           float64
	Scale           float64
	Found           bool
	Duration        time.Duration
	ScalesEvaluated int
}

// MultiScaleMatch is the public, single-call API for multi-scale matching.
// It forwards to the parallel implementation.
func MultiScaleMatch(frame *image.RGBA, tmpl image.Image, opts MultiScaleOptions) MultiScaleResult {
	return MultiScaleMatchParallel(frame, tmpl, opts)
}

// MultiScaleMatchParallel evaluates the template at multiple scales in
// parallel and returns the best match. It supports an optional early-stop
// threshold in MultiScaleOptions.StopOnScore.
func MultiScaleMatchParallel(frame *image.RGBA, tmpl image.Image, opts MultiScaleOptions) MultiScaleResult {
	if frame == nil || tmpl == nil {
		return MultiScaleResult{}
	}

	preGray := buildGrayPrecomp(frame)
	baseTmpl := getTemplatePrecomp(tmpl)
	if baseTmpl == nil {
		return MultiScaleResult{}
	}

	if len(opts.Scales) == 0 {
		if opts.MinScale > 0 && opts.MaxScale > 0 && opts.ScaleStep > 0 && opts.MaxScale >= opts.MinScale {
			maxSteps := 1 + int((opts.MaxScale-opts.MinScale)/opts.ScaleStep+0.5)
			if maxSteps > 200 {
				maxSteps = 200
			}
			scales := make([]ScaleSpec, 0, maxSteps)
			for s := opts.MinScale; s <= opts.MaxScale+1e-9 && len(scales) < maxSteps; s += opts.ScaleStep {
				scales = append(scales, ScaleSpec{Factor: s})
			}
			opts.Scales = scales
		}
	}

	var earlyStop int32
	results := make(chan MultiScaleResult, len(opts.Scales))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	var totalDur int64
	var scalesCount uint64

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
			scaledPc := getScaledTemplatePrecompFromBase(baseTmpl, factor)
			if scaledPc == nil {
				return
			}
			res := matchTemplateNCCGrayIntegralPre(frame, scaledPc, opts.NCC, preGray)
			msr := MultiScaleResult{X: res.X, Y: res.Y, Score: res.Score, Scale: factor, Found: res.Found}
			if opts.NCC.DebugTiming && res.Dur > 0 {
				atomic.AddInt64(&totalDur, res.Dur.Nanoseconds())
			}
			atomic.AddUint64(&scalesCount, 1)
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
		if atomic.LoadInt32(&earlyStop) == 1 && r.Score >= opts.StopOnScore && opts.StopOnScore > 0 {
			break
		}
	}
	dur := atomic.LoadInt64(&totalDur)
	if dur > 0 {
		best.Duration = time.Duration(dur)
	}
	if count := atomic.LoadUint64(&scalesCount); count > 0 {
		best.ScalesEvaluated = int(count)
	}
	return best
}
