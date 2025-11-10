package capture

import (
	"image"
	"math"
	"sync"
	"time"
)

// grayPrecomp stores per-frame grayscale values and their summed-area tables
// (integral images). The integrals allow O(1) window sum and variance queries.
type grayPrecomp struct {
	gray       []float64 // per pixel grayscale (length W*H)
	integral   []float64 // summed-area table of grayscale
	integralSq []float64 // summed-area table of grayscale squared
	W, H       int
}

// templatePrecomp caches grayscale pixels and summary statistics for a
// template (or a scaled version of it).
type templatePrecomp struct {
	gray  []float32
	sumT  float64
	sumT2 float64
	W, H  int
	meanT float64
	stdT  float64
}

// tmplCacheByDim caches templatePrecomp instances by their [width,height].
var (
	tmplCacheMu    sync.RWMutex
	tmplCacheByDim = map[[2]int]*templatePrecomp{}
)

// getTemplatePrecomp returns a cached templatePrecomp for tmpl or builds and
// caches a new one. Pixels with alpha==0 are ignored when computing stats.
func getTemplatePrecomp(tmpl image.Image) *templatePrecomp {
	if tmpl == nil {
		return nil
	}
	b := tmpl.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return nil
	}
	key := [2]int{w, h}
	tmplCacheMu.RLock()
	pc := tmplCacheByDim[key]
	tmplCacheMu.RUnlock()
	if pc != nil {
		return pc
	}
	// Build new precomp
	need := w * h
	gray := make([]float32, need)
	var sumT, sumT2 float64
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bb, a := tmpl.At(b.Min.X+x, b.Min.Y+y).RGBA()
			if a == 0 { // transparent ignored
				continue
			}
			gval := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(bb)
			off := y*w + x
			gray[off] = float32(gval)
			sumT += gval
			sumT2 += gval * gval
		}
	}
	n := float64(need)
	meanT := sumT / n
	varT := (sumT2 - sumT*sumT/n) / n
	stdT := 0.0
	if varT > 0 {
		stdT = math.Sqrt(varT)
	}
	pc = &templatePrecomp{gray: gray, sumT: sumT, sumT2: sumT2, W: w, H: h, meanT: meanT, stdT: stdT}
	tmplCacheMu.Lock()
	// Double-check another goroutine didn't insert meanwhile; keep first to avoid duplicate slices.
	if existing := tmplCacheByDim[key]; existing == nil {
		tmplCacheByDim[key] = pc
	} else {
		pc = existing
	}
	tmplCacheMu.Unlock()
	return pc
}

// getScaledTemplatePrecompFromBase returns a cached or newly built scaled
// templatePrecomp. Scaling is done with bilinear interpolation on the base
// grayscale data to avoid repeated color conversions.
func getScaledTemplatePrecompFromBase(base *templatePrecomp, factor float64) *templatePrecomp {
	if base == nil || factor <= 0 {
		return nil
	}
	if factor == 1.0 {
		return base
	}
	w := int(float64(base.W) * factor)
	h := int(float64(base.H) * factor)
	if w < 2 || h < 2 {
		return nil
	}
	key := [2]int{w, h}
	tmplCacheMu.RLock()
	pc := tmplCacheByDim[key]
	tmplCacheMu.RUnlock()
	if pc != nil {
		return pc
	}
	gray := make([]float32, w*h)
	var sumT, sumT2 float64
	// Precompute inverse factor for coordinate mapping.
	fx := float64(base.W) / float64(w)
	fy := float64(base.H) / float64(h)
	bw := base.W
	bh := base.H
	src := base.gray
	for y := 0; y < h; y++ {
		ys := (float64(y)+0.5)*fy - 0.5
		if ys < 0 {
			ys = 0
		} else if ys > float64(bh-1) {
			ys = float64(bh - 1)
		}
		y0 := int(math.Floor(ys))
		y1 := y0 + 1
		if y1 >= bh {
			y1 = bh - 1
		}
		dy := ys - float64(y0)
		// dy used directly in interpolation (1-dy) and dy; no need to store wy0/wy1.
		for x := 0; x < w; x++ {
			xs := (float64(x)+0.5)*fx - 0.5
			if xs < 0 {
				xs = 0
			} else if xs > float64(bw-1) {
				xs = float64(bw - 1)
			}
			x0 := int(math.Floor(xs))
			x1 := x0 + 1
			if x1 >= bw {
				x1 = bw - 1
			}
			dx := xs - float64(x0)
			wx0 := 1 - dx
			wx1 := dx
			// Bilinear interpolation
			g00 := src[y0*bw+x0]
			g10 := src[y0*bw+x1]
			g01 := src[y1*bw+x0]
			g11 := src[y1*bw+x1]
			top := float64(g00)*wx0 + float64(g10)*wx1
			bottom := float64(g01)*wx0 + float64(g11)*wx1
			gval := float32(top*(1-dy) + bottom*dy)
			off := y*w + x
			gray[off] = gval
			fv := float64(gval)
			sumT += fv
			sumT2 += fv * fv
		}
	}
	n := float64(w * h)
	meanT := sumT / n
	varT := (sumT2 - sumT*sumT/n) / n
	stdT := 0.0
	if varT > 0 {
		stdT = math.Sqrt(varT)
	}
	pc = &templatePrecomp{gray: gray, sumT: sumT, sumT2: sumT2, W: w, H: h, meanT: meanT, stdT: stdT}
	tmplCacheMu.Lock()
	if existing := tmplCacheByDim[key]; existing == nil {
		tmplCacheByDim[key] = pc
	} else {
		pc = existing
	}
	tmplCacheMu.Unlock()
	return pc
}

// matchTemplateNCCGrayIntegralPre computes normalized cross-correlation (NCC)
// between a templatePrecomp and a frame represented by grayPrecomp. It returns
// the best match position and score according to opts.
func matchTemplateNCCGrayIntegralPre(frame *image.RGBA, pc *templatePrecomp, opts NCCOptions, pre *grayPrecomp) NCCResult {
	start := time.Now()
	res := NCCResult{Score: -1}
	if frame == nil || pc == nil || pre == nil {
		return res
	}
	fb := frame.Bounds()
	W, H := fb.Dx(), fb.Dy()
	w, h := pc.W, pc.H
	if w == 0 || h == 0 || W < w || H < h {
		return res
	}
	n := float64(w * h)
	meanT := pc.meanT
	stdT := pc.stdT
	if stdT <= 1e-9 {
		ref := float64(pc.gray[0])
		for y := 0; y <= H-h; y += opts.Stride {
			for x := 0; x <= W-w; x += opts.Stride {
				cy := y + h/2
				cx := x + w/2
				center := pre.gray[cy*W+cx]
				if math.Abs(center-ref) > 1e-9 {
					continue
				}
				ok := true
				for i := 0; i < len(pc.gray); i++ {
					py := i / w
					px := i % w
					if math.Abs(pre.gray[(y+py)*W+(x+px)]-ref) > 1e-9 {
						ok = false
						break
					}
				}
				if ok {
					res.X, res.Y = x+fb.Min.X, y+fb.Min.Y
					res.Score = 1
					res.Found = true
					if opts.DebugTiming {
						res.Dur = time.Since(start)
					}
					return res
				}
			}
		}
		if opts.DebugTiming {
			res.Dur = time.Since(start)
		}
		return res
	}

	bestX, bestY, bestScore := 0, 0, -1.0
	stride := opts.Stride
	if stride <= 0 {
		stride = 1
	}
	for y := 0; y <= H-h; y += stride {
		for x := 0; x <= W-w; x += stride {
			sumF := integralSum(pre.integral, pre.W, x, y, x+w-1, y+h-1)
			sumF2 := integralSum(pre.integralSq, pre.W, x, y, x+w-1, y+h-1)
			meanF := sumF / n
			varF := (sumF2 - sumF*sumF/n) / n
			if varF <= 1e-9 {
				continue
			}
			stdF := math.Sqrt(varF)
			var sumFT float64
			for i := 0; i < len(pc.gray); i++ {
				py := i / w
				px := i % w
				sumFT += pre.gray[(y+py)*W+(x+px)] * float64(pc.gray[i])
			}
			numer := sumFT - n*meanF*meanT
			denom := n * stdF * stdT
			if denom <= 0 {
				continue
			}
			score := numer / denom
			if score > bestScore {
				bestScore, bestX, bestY = score, x, y
			}
		}
	}
	if opts.Refine && stride > 1 {
		minY := max(0, bestY-stride)
		maxY := min(H-h, bestY+stride)
		minX := max(0, bestX-stride)
		maxX := min(W-w, bestX+stride)
		for y := minY; y <= maxY; y++ {
			for x := minX; x <= maxX; x++ {
				sumF := integralSum(pre.integral, pre.W, x, y, x+w-1, y+h-1)
				sumF2 := integralSum(pre.integralSq, pre.W, x, y, x+w-1, y+h-1)
				meanF := sumF / n
				varF := (sumF2 - sumF*sumF/n) / n
				if varF <= 1e-9 {
					continue
				}
				stdF := math.Sqrt(varF)
				var sumFT float64
				for i := 0; i < len(pc.gray); i++ {
					py := i / w
					px := i % w
					sumFT += pre.gray[(y+py)*W+(x+px)] * float64(pc.gray[i])
				}
				numer := sumFT - n*meanF*meanT
				denom := n * stdF * stdT
				if denom <= 0 {
					continue
				}
				score := numer / denom
				if score > bestScore {
					bestScore, bestX, bestY = score, x, y
				}
			}
		}
	}
	res.X, res.Y, res.Score = bestX+fb.Min.X, bestY+fb.Min.Y, bestScore
	res.Found = bestScore >= opts.Threshold
	if !res.Found && opts.ReturnBestEven {
		res.X, res.Y = bestX+fb.Min.X, bestY+fb.Min.Y
	}
	if opts.DebugTiming {
		res.Dur = time.Since(start)
	}
	return res
}

// buildGrayPrecomp computes per-pixel grayscale values and their summed-area
// tables for a frame. Alpha==0 pixels contribute zero.
func buildGrayPrecomp(frame *image.RGBA) *grayPrecomp {
	if frame == nil {
		return nil
	}
	b := frame.Bounds()
	W, H := b.Dx(), b.Dy()
	need := W * H
	p := &grayPrecomp{
		gray:       make([]float64, need),
		integral:   make([]float64, need),
		integralSq: make([]float64, need),
		W:          W,
		H:          H,
	}
	for y := 0; y < H; y++ {
		var rowSum, rowSum2 float64
		for x := 0; x < W; x++ {
			r, gg, bb, a := frame.At(b.Min.X+x, b.Min.Y+y).RGBA()
			var gray float64
			if a != 0 {
				gray = 0.2126*float64(r) + 0.7152*float64(gg) + 0.0722*float64(bb)
			}
			off := y*W + x
			p.gray[off] = gray
			rowSum += gray
			rowSum2 += gray * gray
			if y == 0 {
				p.integral[off] = rowSum
				p.integralSq[off] = rowSum2
			} else {
				p.integral[off] = p.integral[(y-1)*W+x] + rowSum
				p.integralSq[off] = p.integralSq[(y-1)*W+x] + rowSum2
			}
		}
	}
	return p
}

// integralSum returns the inclusive sum over rectangle [x0..x1] x [y0..y1]
// from an integral image stored in row-major order with width W.
func integralSum(I []float64, W int, x0, y0, x1, y1 int) float64 {
	if x0 > x1 || y0 > y1 {
		return 0
	}
	A := func(x, y int) float64 {
		if x < 0 || y < 0 {
			return 0
		}
		return I[y*W+x]
	}
	return A(x1, y1) - A(x0-1, y1) - A(x1, y0-1) + A(x0-1, y0-1)
}

// NCCOptions configures normalized cross-correlation template matching.
type NCCOptions struct {
	Threshold      float64 // Minimum NCC score for a positive match (default 0.80)
	Stride         int     // Coarse stride for scanning (default 1)
	Refine         bool    // If true and Stride>1, do a refinement pass around best window
	ReturnBestEven bool    // If true, Found=false but best coordinates returned even if below threshold
	DebugTiming    bool    // If true, measure elapsed time (no logging here; hook point)
}

// NCCResult holds the outcome of a template matching operation.
type NCCResult struct {
	X, Y  int
	Score float64
	Found bool
	Dur   time.Duration // Only set if DebugTiming
}

// MatchTemplateNCC performs masked NCC on RGBA images. Template pixels with
// alpha==0 are ignored; frame alpha==0 pixels contribute zero. It returns
// the best match according to Threshold and Stride options.
func MatchTemplateNCC(frame *image.RGBA, tmpl image.Image, opts NCCOptions) NCCResult {
	if opts.Threshold <= 0 {
		opts.Threshold = 0.80
	}
	if opts.Stride <= 0 {
		opts.Stride = 1
	}
	if frame == nil || tmpl == nil {
		return NCCResult{Score: -1}
	}
	fb := frame.Bounds()
	tb := tmpl.Bounds()
	if tb.Dx() == 0 || tb.Dy() == 0 || fb.Dx() < tb.Dx() || fb.Dy() < tb.Dy() {
		return NCCResult{Score: -1}
	}
	pre := buildGrayPrecomp(frame)
	if pre == nil {
		return NCCResult{Score: -1}
	}
	pc := getTemplatePrecomp(tmpl)
	res := matchTemplateNCCGrayIntegralPre(frame, pc, opts, pre)
	return res
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
