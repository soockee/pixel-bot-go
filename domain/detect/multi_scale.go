package detect

import (
	"image"
	"sync"
)

// scaledTemplate is a template prepared at one scale factor.
type scaledTemplate struct {
	factor float64
	pc     *templatePrecomp
}

// MultiScaleResult is the best match found across scales.
type MultiScaleResult struct {
	X, Y  int
	Score float64
	Scale float64
	Found bool
}

// scaleRange lists the factors from minScale to maxScale, at most 200 of them.
func scaleRange(minScale, maxScale, step float64) []float64 {
	var scales []float64
	for i := range 200 {
		s := minScale + float64(i)*step
		if s > maxScale+1e-9 {
			break
		}
		scales = append(scales, s)
	}
	return scales
}

// prepareTemplates builds tmpl at each scale, skipping scales too small to match.
func prepareTemplates(tmpl image.Image, scales []float64) []scaledTemplate {
	base := newTemplatePrecomp(tmpl)
	var out []scaledTemplate
	for _, s := range scales {
		if pc := scaledTemplatePrecomp(base, s); pc != nil {
			out = append(out, scaledTemplate{factor: s, pc: pc})
		}
	}
	return out
}

// matchScales runs every template in parallel and returns the best match. The
// first template wins a tie, so the same frame always gives the same result.
func matchScales(pre *grayPrecomp, templates []scaledTemplate, opts NCCOptions) MultiScaleResult {
	results := make([]NCCResult, len(templates))
	var wg sync.WaitGroup
	for i, t := range templates {
		wg.Go(func() { results[i] = matchTemplate(pre, t.pc, opts) })
	}
	wg.Wait()

	best := MultiScaleResult{Score: -1}
	for i, r := range results {
		if r.Score > best.Score {
			best = MultiScaleResult{X: r.X, Y: r.Y, Score: r.Score, Scale: templates[i].factor, Found: r.Found}
		}
	}
	return best
}
