package capture

import (
	"image"
	"math"
	"time"
)

// grayPrecomp holds precomputed grayscale pixel values and integral images (summed-area tables)
// for the current frame to allow O(1) mean/variance queries per window. Dot product still
// requires O(n) over template pixels. Only used for fully opaque grayscale template path.
type grayPrecomp struct {
	gray       []float64 // per pixel grayscale (length W*H)
	integral   []float64 // summed-area table of grayscale
	integralSq []float64 // summed-area table of grayscale squared
	W, H       int
}

// buildGrayPrecomp constructs grayscale arrays and integral images for a frame.
func buildGrayPrecomp(frame *image.RGBA) *grayPrecomp {
	if frame == nil {
		return nil
	}
	b := frame.Bounds()
	W, H := b.Dx(), b.Dy()
	g := make([]float64, W*H)
	I := make([]float64, W*H)
	I2 := make([]float64, W*H)
	for y := 0; y < H; y++ {
		var rowSum, rowSum2 float64
		for x := 0; x < W; x++ {
			r, gg, bb, a := frame.At(b.Min.X+x, b.Min.Y+y).RGBA()
			var gray float64
			if a != 0 { // treat transparent as 0 contribution
				gray = 0.2126*float64(r) + 0.7152*float64(gg) + 0.0722*float64(bb)
			}
			off := y*W + x
			g[off] = gray
			rowSum += gray
			rowSum2 += gray * gray
			if y == 0 {
				I[off] = rowSum
				I2[off] = rowSum2
			} else {
				I[off] = I[(y-1)*W+x] + rowSum
				I2[off] = I2[(y-1)*W+x] + rowSum2
			}
		}
	}
	return &grayPrecomp{gray: g, integral: I, integralSq: I2, W: W, H: H}
}

// integralSum returns sum over rectangle [x0,x1]x[y0,y1] inclusive.
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

// isTemplateFullyOpaque scans template alpha and returns true if all pixels are fully opaque (a != 0).
func isTemplateFullyOpaque(tmpl image.Image) bool {
	if tmpl == nil {
		return false
	}
	b := tmpl.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := tmpl.At(x, y).RGBA()
			if a == 0 {
				return false
			}
		}
	}
	return true
}

// matchTemplateNCCGrayIntegral performs NCC using precomputed grayscale + integral tables assuming
// fully opaque template (all pixels relevant). Falls back to standard path if template constant.
func matchTemplateNCCGrayIntegral(frame *image.RGBA, tmpl image.Image, opts NCCOptions, pre *grayPrecomp) NCCResult {
	start := time.Now()
	res := NCCResult{Score: -1}
	if frame == nil || tmpl == nil || pre == nil {
		return res
	}
	fb := frame.Bounds()
	tb := tmpl.Bounds()
	W, H := fb.Dx(), fb.Dy()
	w, h := tb.Dx(), tb.Dy()
	if w == 0 || h == 0 || W < w || H < h {
		return res
	}

	// Build template grayscale + stats
	tGray := make([]float64, w*h)
	var sumT, sumT2 float64
	for ty := 0; ty < h; ty++ {
		for tx := 0; tx < w; tx++ {
			r, g, b, a := tmpl.At(tb.Min.X+tx, tb.Min.Y+ty).RGBA()
			if a == 0 { // Should not happen if fully opaque, but guard
				continue
			}
			gray := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
			off := ty*w + tx
			tGray[off] = gray
			sumT += gray
			sumT2 += gray * gray
		}
	}
	n := float64(w * h)
	meanT := sumT / n
	varT := (sumT2 - sumT*sumT/n) / n
	if varT <= 1e-9 { // constant template equality shortcut (check multiple pixels)
		ref := tGray[0]
		for y := 0; y <= H-h; y += opts.Stride {
			for x := 0; x <= W-w; x += opts.Stride {
				// Quick check center pixel equality
				cy := y + h/2
				cx := x + w/2
				center := pre.gray[cy*W+cx]
				if math.Abs(center-ref) > 1e-9 {
					continue
				}
				// Verify all pixels (early break)
				ok := true
				for i := 0; i < len(tGray); i++ {
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
	stdT := math.Sqrt(varT)

	bestX, bestY, bestScore := 0, 0, -1.0
	stride := opts.Stride
	if stride <= 0 {
		stride = 1
	}
	for y := 0; y <= H-h; y += stride {
		for x := 0; x <= W-w; x += stride {
			// Mean & variance via integrals
			sumF := integralSum(pre.integral, pre.W, x, y, x+w-1, y+h-1)
			sumF2 := integralSum(pre.integralSq, pre.W, x, y, x+w-1, y+h-1)
			meanF := sumF / n
			varF := (sumF2 - sumF*sumF/n) / n
			if varF <= 1e-9 {
				continue
			}
			stdF := math.Sqrt(varF)
			// Dot product Σ F_i * T_i
			var sumFT float64
			for i := 0; i < len(tGray); i++ {
				py := i / w
				px := i % w
				sumFT += pre.gray[(y+py)*W+(x+px)] * tGray[i]
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
	// Optional refinement: treat same as original (not integral optimized) for simplicity.
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
				for i := 0; i < len(tGray); i++ {
					py := i / w
					px := i % w
					sumFT += pre.gray[(y+py)*W+(x+px)] * tGray[i]
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

// Dummy references to ensure package-level use (in case cross-file references are misdetected by linter).
var _ = grayPrecomp{}
var _ = integralSum

// NCCOptions configures the normalized cross correlation matching.
type NCCOptions struct {
	Threshold      float64 // Minimum NCC score for a positive match (default 0.80)
	Stride         int     // Coarse stride for scanning (default 1)
	Refine         bool    // If true and Stride>1, do a refinement pass around best window
	ReturnBestEven bool    // If true, Found=false but best coordinates returned even if below threshold
	DebugTiming    bool    // If true, measure elapsed time (no logging here; hook point)
	UseRGB         bool    // If true, perform NCC over RGB channels (averaged) instead of grayscale
}

// NCCResult holds the outcome of template matching.
type NCCResult struct {
	X, Y  int
	Score float64
	Found bool
	Dur   time.Duration // Only set if DebugTiming
}

// MatchTemplateNCC performs masked normalized cross-correlation on RGBA images.
// Transparency (alpha==0) in the template is ignored. Frame pixels with alpha==0
// contribute zero. Returns best match respecting Threshold and Stride.
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
	// Dispatch paths
	if opts.UseRGB {
		return matchTemplateNCCRBG(frame, tmpl, opts)
	}
	if isTemplateFullyOpaque(tmpl) { // try integral accelerated path
		pre := buildGrayPrecomp(frame)
		if pre != nil {
			return matchTemplateNCCGrayIntegral(frame, tmpl, opts, pre)
		}
	}
	// Fallback: use RGB path for templates with transparency instead of separate grayscale masked path.
	return matchTemplateNCCRBG(frame, tmpl, opts)
}

// matchTemplateNCCRBG handles RGB averaged NCC with masking.
func matchTemplateNCCRBG(frame *image.RGBA, tmpl image.Image, opts NCCOptions) NCCResult {
	start := time.Now()
	fb := frame.Bounds()
	tb := tmpl.Bounds()
	W, H := fb.Dx(), fb.Dy()
	w, h := tb.Dx(), tb.Dy()
	res := NCCResult{Score: -1}
	indices := make([]int, 0, w*h)
	tR := make([]float64, 0, w*h)
	tG := make([]float64, 0, w*h)
	tB := make([]float64, 0, w*h)
	var sumTR, sumTR2, sumTG, sumTG2, sumTB, sumTB2 float64
	for ty := 0; ty < h; ty++ {
		for tx := 0; tx < w; tx++ {
			r, g, b, a := tmpl.At(tb.Min.X+tx, tb.Min.Y+ty).RGBA()
			if a == 0 {
				continue
			}
			off := ty*w + tx
			indices = append(indices, off)
			rv, gv, bv := float64(r), float64(g), float64(b)
			tR = append(tR, rv)
			tG = append(tG, gv)
			tB = append(tB, bv)
			sumTR += rv
			sumTR2 += rv * rv
			sumTG += gv
			sumTG2 += gv * gv
			sumTB += bv
			sumTB2 += bv * bv
		}
	}
	n := float64(len(indices))
	if n == 0 {
		return res
	}
	meanTR := sumTR / n
	varTR := (sumTR2 - sumTR*sumTR/n) / n
	meanTG := sumTG / n
	varTG := (sumTG2 - sumTG*sumTG/n) / n
	meanTB := sumTB / n
	varTB := (sumTB2 - sumTB*sumTB/n) / n
	if varTR <= 1e-9 && varTG <= 1e-9 && varTB <= 1e-9 { // constant template equality shortcut (red channel basis)
		refR := tR[0]
		for y := fb.Min.Y; y <= fb.Max.Y-h; y += opts.Stride {
			for x := fb.Min.X; x <= fb.Max.X-w; x += opts.Stride {
				r0, _, _, a0 := frame.At(x, y).RGBA()
				if a0 == 0 {
					continue
				}
				if math.Abs(float64(r0)-refR) < 1e-9 {
					res.X, res.Y, res.Score, res.Found = x, y, 1, true
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
	stdTR := math.Sqrt(varTR)
	stdTG := math.Sqrt(varTG)
	stdTB := math.Sqrt(varTB)

	fR := make([]float64, W*H)
	fG := make([]float64, W*H)
	fB := make([]float64, W*H)
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			r, g, b, a := frame.At(fb.Min.X+x, fb.Min.Y+y).RGBA()
			if a == 0 {
				continue
			}
			off := y*W + x
			fR[off] = float64(r)
			fG[off] = float64(g)
			fB[off] = float64(b)
		}
	}
	bestX, bestY, bestScore := 0, 0, -1.0
	stride := opts.Stride
	for y := 0; y <= H-h; y += stride {
		for x := 0; x <= W-w; x += stride {
			var sumFR, sumFR2, sumFTR float64
			var sumFG, sumFG2, sumFTG float64
			var sumFB, sumFB2, sumFTB float64
			for i, off := range indices {
				py := off / w
				px := off % w
				idx := (y+py)*W + (x + px)
				vr := fR[idx]
				vg := fG[idx]
				vb := fB[idx]
				sumFR += vr
				sumFR2 += vr * vr
				sumFTR += vr * tR[i]
				sumFG += vg
				sumFG2 += vg * vg
				sumFTG += vg * tG[i]
				sumFB += vb
				sumFB2 += vb * vb
				sumFTB += vb * tB[i]
			}
			meanFR := sumFR / n
			varFR := (sumFR2 - sumFR*sumFR/n) / n
			meanFG := sumFG / n
			varFG := (sumFG2 - sumFG*sumFG/n) / n
			meanFB := sumFB / n
			varFB := (sumFB2 - sumFB*sumFB/n) / n
			if varFR <= 1e-9 || varFG <= 1e-9 || varFB <= 1e-9 {
				continue
			}
			stdFR := math.Sqrt(varFR)
			stdFG := math.Sqrt(varFG)
			stdFB := math.Sqrt(varFB)
			nR := sumFTR - n*meanFR*meanTR
			dR := n * stdFR * stdTR
			nG := sumFTG - n*meanFG*meanTG
			dG := n * stdFG * stdTG
			nB := sumFTB - n*meanFB*meanTB
			dB := n * stdFB * stdTB
			if dR <= 0 || dG <= 0 || dB <= 0 {
				continue
			}
			score := (nR/dR + nG/dG + nB/dB) / 3.0
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
				var sumFR, sumFR2, sumFTR float64
				var sumFG, sumFG2, sumFTG float64
				var sumFB, sumFB2, sumFTB float64
				for i, off := range indices {
					py := off / w
					px := off % w
					idx := (y+py)*W + (x + px)
					vr := fR[idx]
					vg := fG[idx]
					vb := fB[idx]
					sumFR += vr
					sumFR2 += vr * vr
					sumFTR += vr * tR[i]
					sumFG += vg
					sumFG2 += vg * vg
					sumFTG += vg * tG[i]
					sumFB += vb
					sumFB2 += vb * vb
					sumFTB += vb * tB[i]
				}
				meanFR := sumFR / n
				varFR := (sumFR2 - sumFR*sumFR/n) / n
				meanFG := sumFG / n
				varFG := (sumFG2 - sumFG*sumFG/n) / n
				meanFB := sumFB / n
				varFB := (sumFB2 - sumFB*sumFB/n) / n
				if varFR <= 1e-9 || varFG <= 1e-9 || varFB <= 1e-9 {
					continue
				}
				stdFR := math.Sqrt(varFR)
				stdFG := math.Sqrt(varFG)
				stdFB := math.Sqrt(varFB)
				nR := sumFTR - n*meanFR*meanTR
				dR := n * stdFR * stdTR
				nG := sumFTG - n*meanFG*meanTG
				dG := n * stdFG * stdTG
				nB := sumFTB - n*meanFB*meanTB
				dB := n * stdFB * stdTB
				if dR <= 0 || dG <= 0 || dB <= 0 {
					continue
				}
				score := (nR/dR + nG/dG + nB/dB) / 3.0
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

// matchTemplateNCCGrayMasked handles original masked grayscale NCC path.

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
