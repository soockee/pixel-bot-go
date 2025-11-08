package capture

import (
	"image"
	"math"
	"time"
)

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
	start := time.Now()
	if opts.Threshold <= 0 {
		opts.Threshold = 0.80
	}
	if opts.Stride <= 0 {
		opts.Stride = 1
	}

	res := NCCResult{X: 0, Y: 0, Score: -1, Found: false}
	if frame == nil || tmpl == nil {
		return res
	}
	fb := frame.Bounds()
	tb := tmpl.Bounds()
	W, H := fb.Dx(), fb.Dy()
	w, h := tb.Dx(), tb.Dy()
	if w == 0 || h == 0 || W < w || H < h {
		return res
	}

	// Mask + template statistics preparation.
	indices := make([]int, 0, w*h) // linear offsets of valid (non-transparent) pixels
	if opts.UseRGB {
		// RGB channel arrays for template
		tR := make([]float64, 0, w*h)
		tG := make([]float64, 0, w*h)
		tB := make([]float64, 0, w*h)
		var sumTR, sumTR2 float64
		var sumTG, sumTG2 float64
		var sumTB, sumTB2 float64
		for ty := 0; ty < h; ty++ {
			for tx := 0; tx < w; tx++ {
				c := tmpl.At(tb.Min.X+tx, tb.Min.Y+ty)
				r, g, b, a := c.RGBA()
				if a == 0 {
					continue
				}
				indices = append(indices, ty*w+tx)
				rv := float64(r)
				gv := float64(g)
				bv := float64(b)
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
		n := len(indices)
		if n == 0 {
			return res
		}
		meanTR := sumTR / float64(n)
		meanTG := sumTG / float64(n)
		meanTB := sumTB / float64(n)
		varTR := (sumTR2 - sumTR*sumTR/float64(n)) / float64(n)
		varTG := (sumTG2 - sumTG*sumTG/float64(n)) / float64(n)
		varTB := (sumTB2 - sumTB*sumTB/float64(n)) / float64(n)
		// If template nearly constant across all channels, fall back to simple equality on red.
		if varTR <= 1e-9 && varTG <= 1e-9 && varTB <= 1e-9 {
			refR := tR[0]
			for y := fb.Min.Y; y <= fb.Max.Y-h; y += opts.Stride {
				for x := fb.Min.X; x <= fb.Max.X-w; x += opts.Stride {
					r0, _, _, a0 := frame.At(x, y).RGBA()
					if a0 == 0 {
						continue
					}
					if math.Abs(float64(r0)-refR) < 1e-9 {
						res.X, res.Y = x, y
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
		stdTR := math.Sqrt(varTR)
		stdTG := math.Sqrt(varTG)
		stdTB := math.Sqrt(varTB)

		// Precompute frame channels
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
		// Coarse pass
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
				n := float64(len(indices))
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
		// Refinement pass
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
					n := float64(len(indices))
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
			res.X, res.Y = bestX, bestY
		}
		if opts.DebugTiming {
			res.Dur = time.Since(start)
		}
		return res
	}

	// Grayscale path (original implementation) if UseRGB is false.
	tGray := make([]float64, 0, w*h)
	var sumT, sumT2 float64
	for ty := 0; ty < h; ty++ {
		for tx := 0; tx < w; tx++ {
			c := tmpl.At(tb.Min.X+tx, tb.Min.Y+ty)
			r, g, b, a := c.RGBA()
			if a == 0 {
				continue
			}
			gray := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
			indices = append(indices, ty*w+tx)
			tGray = append(tGray, gray)
			sumT += gray
			sumT2 += gray * gray
		}
	}
	n := len(indices)
	if n == 0 {
		return res
	}
	meanT := sumT / float64(n)
	varT := (sumT2 - sumT*sumT/float64(n)) / float64(n)
	if varT <= 1e-9 { // constant template handled with simple equality fallback
		ref := tGray[0]
		for y := fb.Min.Y; y <= fb.Max.Y-h; y += opts.Stride {
			for x := fb.Min.X; x <= fb.Max.X-w; x += opts.Stride {
				r0, g0, b0, a0 := frame.At(x, y).RGBA()
				if a0 == 0 {
					continue
				}
				gray := 0.2126*float64(r0) + 0.7152*float64(g0) + 0.0722*float64(b0)
				if math.Abs(gray-ref) < 1e-9 {
					res.X, res.Y = x, y
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

	fGray := make([]float64, W*H)
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			r, g, b, a := frame.At(fb.Min.X+x, fb.Min.Y+y).RGBA()
			if a == 0 { // treat transparent as zero
				continue
			}
			fGray[y*W+x] = 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
		}
	}

	bestX, bestY, bestScore := 0, 0, -1.0
	stride := opts.Stride

	// Coarse pass.
	for y := 0; y <= H-h; y += stride {
		for x := 0; x <= W-w; x += stride {
			var sumF, sumF2, sumFT float64
			for i, off := range indices {
				py := off / w
				px := off % w
				val := fGray[(y+py)*W+(x+px)]
				sumF += val
				sumF2 += val * val
				sumFT += val * tGray[i]
			}
			meanF := sumF / float64(n)
			varF := (sumF2 - sumF*sumF/float64(n)) / float64(n)
			if varF <= 1e-9 {
				continue
			}
			stdF := math.Sqrt(varF)
			numer := sumFT - float64(n)*meanF*meanT
			denom := float64(n) * stdF * stdT
			if denom <= 0 {
				continue
			}
			score := numer / denom
			if score > bestScore {
				bestScore, bestX, bestY = score, x, y
			}
		}
	}

	// Refinement pass if requested and stride>1.
	if opts.Refine && stride > 1 {
		minY := max(0, bestY-stride)
		maxY := min(H-h, bestY+stride)
		minX := max(0, bestX-stride)
		maxX := min(W-w, bestX+stride)
		for y := minY; y <= maxY; y++ {
			for x := minX; x <= maxX; x++ {
				var sumF, sumF2, sumFT float64
				for i, off := range indices {
					py := off / w
					px := off % w
					val := fGray[(y+py)*W+(x+px)]
					sumF += val
					sumF2 += val * val
					sumFT += val * tGray[i]
				}
				meanF := sumF / float64(n)
				varF := (sumF2 - sumF*sumF/float64(n)) / float64(n)
				if varF <= 1e-9 {
					continue
				}
				stdF := math.Sqrt(varF)
				numer := sumFT - float64(n)*meanF*meanT
				denom := float64(n) * stdF * stdT
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
		res.X, res.Y = bestX, bestY
	}
	if opts.DebugTiming {
		res.Dur = time.Since(start)
	}
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
