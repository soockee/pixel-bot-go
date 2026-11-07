package detect

import (
	"image"
	"math"
)

// grayPrecomp stores per-frame grayscale values and their summed-area tables
// (integral images), which give O(1) window sum and variance queries. Its
// buffers are reused between frames.
type grayPrecomp struct {
	gray       []float64 // per pixel grayscale (length W*H)
	integral   []float64 // summed-area table of grayscale
	integralSq []float64 // summed-area table of grayscale squared
	W, H       int
}

// load fills the tables from frame, whose pixels are assumed opaque.
func (p *grayPrecomp) load(frame *image.RGBA) {
	b := frame.Bounds()
	p.W, p.H = b.Dx(), b.Dy()
	if need := p.W * p.H; cap(p.gray) < need {
		p.gray = make([]float64, need)
		p.integral = make([]float64, need)
		p.integralSq = make([]float64, need)
	} else {
		p.gray, p.integral, p.integralSq = p.gray[:need], p.integral[:need], p.integralSq[:need]
	}
	for y := range p.H {
		pix := frame.Pix[frame.PixOffset(b.Min.X, b.Min.Y+y):]
		var rowSum, rowSum2 float64
		for x := range p.W {
			rgb := pix[x*4 : x*4+3]
			gray := luminance(uint32(rgb[0])*257, uint32(rgb[1])*257, uint32(rgb[2])*257)
			off := y*p.W + x
			p.gray[off] = gray
			rowSum += gray
			rowSum2 += gray * gray
			if y == 0 {
				p.integral[off] = rowSum
				p.integralSq[off] = rowSum2
			} else {
				p.integral[off] = p.integral[off-p.W] + rowSum
				p.integralSq[off] = p.integralSq[off-p.W] + rowSum2
			}
		}
	}
}

// luminance takes 16-bit channels, as color.Color.RGBA returns them.
func luminance(r, g, b uint32) float64 {
	return 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
}

// templatePrecomp holds grayscale pixels and statistics for a template (or a
// scaled version of it).
type templatePrecomp struct {
	gray  []float32
	W, H  int
	meanT float64
	stdT  float64
}

func newPrecomp(gray []float32, w, h int) *templatePrecomp {
	var sum, sum2 float64
	for _, v := range gray {
		sum += float64(v)
		sum2 += float64(v) * float64(v)
	}
	n := float64(len(gray))
	return &templatePrecomp{gray: gray, W: w, H: h, meanT: sum / n, stdT: math.Sqrt(max((sum2-sum*sum/n)/n, 0))}
}

// newTemplatePrecomp builds a templatePrecomp for tmpl. Pixels with alpha==0
// count as black.
func newTemplatePrecomp(tmpl image.Image) *templatePrecomp {
	b := tmpl.Bounds()
	w, h := b.Dx(), b.Dy()
	gray := make([]float32, w*h)
	for y := range h {
		for x := range w {
			if r, g, bl, a := tmpl.At(b.Min.X+x, b.Min.Y+y).RGBA(); a != 0 {
				gray[y*w+x] = float32(luminance(r, g, bl))
			}
		}
	}
	return newPrecomp(gray, w, h)
}

// scaledTemplatePrecomp returns base scaled by factor, or nil if too small.
// Scaling is done with bilinear interpolation on the base grayscale data to
// avoid repeated color conversions.
func scaledTemplatePrecomp(base *templatePrecomp, factor float64) *templatePrecomp {
	if factor == 1.0 {
		return base
	}
	w := int(float64(base.W) * factor)
	h := int(float64(base.H) * factor)
	if w < 2 || h < 2 {
		return nil
	}
	gray := make([]float32, w*h)
	fx := float64(base.W) / float64(w)
	fy := float64(base.H) / float64(h)
	bw, bh := base.W, base.H
	src := base.gray
	for y := range h {
		ys := min(max((float64(y)+0.5)*fy-0.5, 0), float64(bh-1))
		y0 := int(math.Floor(ys))
		y1 := min(y0+1, bh-1)
		dy := ys - float64(y0)
		for x := range w {
			xs := min(max((float64(x)+0.5)*fx-0.5, 0), float64(bw-1))
			x0 := int(math.Floor(xs))
			x1 := min(x0+1, bw-1)
			dx := xs - float64(x0)
			top := float64(src[y0*bw+x0])*(1-dx) + float64(src[y0*bw+x1])*dx
			bottom := float64(src[y1*bw+x0])*(1-dx) + float64(src[y1*bw+x1])*dx
			gray[y*w+x] = float32(top*(1-dy) + bottom*dy)
		}
	}
	return newPrecomp(gray, w, h)
}

// NCCOptions configures normalized cross-correlation template matching.
type NCCOptions struct {
	Threshold float64 // Minimum NCC score for a positive match
	Stride    int     // Coarse step for scanning
	Refine    bool    // If true and Stride>1, rescan around the best coarse position
}

// NCCResult holds the outcome of a template matching operation. X and Y are the
// top-left corner of the best position.
type NCCResult struct {
	X, Y  int
	Score float64
	Found bool
}

// matchTemplate returns the position where the template correlates best with
// the frame in pre.
func matchTemplate(pre *grayPrecomp, pc *templatePrecomp, opts NCCOptions) NCCResult {
	res := NCCResult{Score: -1}
	if pre.W < pc.W || pre.H < pc.H {
		return res
	}
	// A coarse step larger than a quarter of the template's short side can step
	// over the true position (the score there drops below noise), and refine
	// then polishes the wrong spot. Keep the step small enough to land close.
	stride := min(max(opts.Stride, 1), max(min(pc.W, pc.H)/4, 1))
	lastX, lastY := pre.W-pc.W, pre.H-pc.H
	pc.scan(pre, image.Rect(0, 0, lastX, lastY), stride, &res)
	if opts.Refine && stride > 1 {
		near := image.Rectangle{
			Min: image.Pt(max(res.X-stride, 0), max(res.Y-stride, 0)),
			Max: image.Pt(min(res.X+stride, lastX), min(res.Y+stride, lastY)),
		}
		pc.scan(pre, near, 1, &res)
	}
	res.Found = res.Score >= opts.Threshold
	return res
}

// scan scores the positions in area, ends included, and updates best.
func (pc *templatePrecomp) scan(pre *grayPrecomp, area image.Rectangle, step int, best *NCCResult) {
	for y := area.Min.Y; y <= area.Max.Y; y += step {
		for x := area.Min.X; x <= area.Max.X; x += step {
			if score, ok := pc.scoreAt(pre, x, y); ok && score > best.Score {
				best.X, best.Y, best.Score = x, y, score
			}
		}
	}
}

// scoreAt computes the NCC score with the template's top-left corner at (x, y).
// It reports false when either side has no contrast.
func (pc *templatePrecomp) scoreAt(pre *grayPrecomp, x, y int) (float64, bool) {
	w, h := pc.W, pc.H
	n := float64(w * h)
	sumF := integralSum(pre.integral, pre.W, x, y, x+w-1, y+h-1)
	sumF2 := integralSum(pre.integralSq, pre.W, x, y, x+w-1, y+h-1)
	meanF := sumF / n
	varF := (sumF2 - sumF*sumF/n) / n
	if varF <= 1e-9 {
		return 0, false
	}
	denom := n * math.Sqrt(varF) * pc.stdT
	if denom <= 0 {
		return 0, false
	}
	var sumFT float64
	for py := range h {
		frameRow := pre.gray[(y+py)*pre.W+x:][:w]
		tmplRow := pc.gray[py*w:][:w]
		for i, v := range frameRow {
			sumFT += v * float64(tmplRow[i])
		}
	}
	return (sumFT - n*meanF*pc.meanT) / denom, true
}

// integralSum returns the inclusive sum over rectangle [x0..x1] x [y0..y1]
// from an integral image stored in row-major order with width W.
func integralSum(I []float64, W int, x0, y0, x1, y1 int) float64 {
	if x0 > x1 || y0 > y1 {
		return 0
	}
	sum := I[y1*W+x1]
	if x0 > 0 {
		sum -= I[y1*W+x0-1]
	}
	if y0 > 0 {
		sum -= I[(y0-1)*W+x1]
	}
	if x0 > 0 && y0 > 0 {
		sum += I[(y0-1)*W+x0-1]
	}
	return sum
}
