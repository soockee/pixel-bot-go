package detect

import (
	"cmp"
	"image"
	"math"
	"slices"
)

const (
	colorRedMin      = 30  // R-(G+B) of a red feather pixel
	colorBlueMin     = 30  // B-(R+G) of a blue feather pixel
	colorTanMin      = 250 // tan score of the bobber's wooden body, see tanScore
	colorPairDist2   = 626 // squared distance in pixels within which two parts form one bobber
	colorMinContrast = 15  // CIELAB distance from the water a bobber part needs
	colorWaterSpread = 20  // CIELAB distance from the median that still counts as water
	colorMaxTop      = 160 // most candidates per class before giving up
)

// ColorLocator finds the bobber without the addon, from the colours of its
// parts: a red and a blue feather and a tan body. It takes the strongest pixels
// of each colour, and two parts of different colours close together that stand
// out from the water are the bobber. It is weaker than MarkerLocator: other
// players' bobbers, nameplates and spell effects can fool it.
type ColorLocator struct{}

// NewColorLocator returns a ColorLocator.
func NewColorLocator() *ColorLocator { return &ColorLocator{} }

type colorClass int

const (
	classRed colorClass = iota
	classBlue
	classTan
)

type candidate struct {
	x, y  int
	score float64
	lab   [3]float64
}

// Locate searches the band above the player's character for the bobber's
// colours. Score is the parts' contrast to the water, 1 at 100 CIELAB units.
func (l *ColorLocator) Locate(frame *image.RGBA) (Match, bool) {
	return l.locate(frame, image.Rectangle{})
}

func (l *ColorLocator) locate(frame *image.RGBA, exclude image.Rectangle) (Match, bool) {
	area := searchArea(frame.Bounds())
	water, ok := waterColor(frame, area)
	if !ok {
		return Match{}, false
	}
	var classes [3][]candidate
	for y := area.Min.Y; y < area.Max.Y; y++ {
		row := frame.Pix[frame.PixOffset(area.Min.X, y):]
		for i := range area.Dx() {
			x := area.Min.X + i
			if image.Pt(x, y).In(exclude) {
				continue
			}
			p := row[i*4:]
			r, g, b := int(p[0]), int(p[1]), int(p[2])
			if r|g|b == 0 {
				continue // recordings black out everything but the marker box
			}
			if s := r - g - b; s >= colorRedMin {
				classes[classRed] = append(classes[classRed], candidate{x: x, y: y, score: float64(s)})
			}
			if s := b - r - g; s >= colorBlueMin {
				classes[classBlue] = append(classes[classBlue], candidate{x: x, y: y, score: float64(s)})
			}
			if s := tanScore(r, g, b); s >= colorTanMin {
				classes[classTan] = append(classes[classTan], candidate{x: x, y: y, score: s})
			}
		}
	}
	for c := range classes {
		slices.SortFunc(classes[c], func(a, b candidate) int { return cmp.Compare(b.score, a.score) })
		classes[c] = classes[c][:min(len(classes[c]), colorMaxTop)]
		for i, cand := range classes[c] {
			p := frame.RGBAAt(cand.x, cand.y)
			classes[c][i].lab = lab(p.R, p.G, p.B)
		}
	}
	// Few strong candidates first, so the bobber's brightest parts win over
	// weak matches elsewhere.
	for n := 5; n <= colorMaxTop; n *= 2 {
		if m, ok := bestPair(classes, n, water, frame.Bounds()); ok {
			return m, true
		}
	}
	return Match{}, false
}

// searchArea is the band where the bobber lands: above the character, who
// stands at the bottom centre, and inside the edges where the interface sits.
// shortcut: a fixed band, so a camera tilted far down or a bobber cast to the
// edge is missed; upgrade to a band learned from the casts' marker boxes.
func searchArea(r image.Rectangle) image.Rectangle {
	w, h := r.Dx(), r.Dy()
	return image.Rect(r.Min.X+w/10, r.Min.Y+h/20, r.Max.X-w/10, r.Min.Y+h*7/10)
}

// bestPair returns the two pixels of different classes among the top n of each
// class that lie close together and stand out most from the water.
func bestPair(classes [3][]candidate, n int, water [3]float64, frame image.Rectangle) (Match, bool) {
	var best Match
	found := false
	for i := range classes {
		for j := i + 1; j < len(classes); j++ {
			for _, a := range classes[i][:min(n, len(classes[i]))] {
				ca := deltaE(a.lab, water)
				if ca < colorMinContrast {
					continue
				}
				for _, b := range classes[j][:min(n, len(classes[j]))] {
					dx, dy := a.x-b.x, a.y-b.y
					cb := deltaE(b.lab, water)
					if dx*dx+dy*dy >= colorPairDist2 || cb < colorMinContrast {
						continue
					}
					if score := (ca + cb) / 200; score > best.Score {
						best, found = pairMatch(a, b, score, frame), true
					}
				}
			}
		}
	}
	return best, found
}

// pairMatch centres the watched box on the two parts.
// shortcut: the box side is a fixed share of the frame height, as the bobber's
// size on screen follows the resolution; upgrade to sizing it from the parts.
func pairMatch(a, b candidate, score float64, frame image.Rectangle) Match {
	x, y := (a.x+b.x)/2-frame.Min.X, (a.y+b.y)/2-frame.Min.Y
	half := max(markerMinSide, frame.Dy()/8) / 2
	box := image.Rect(x-half, y-half, x+half, y+half).Intersect(frame.Sub(frame.Min))
	return Match{X: x, Y: y, Box: box, Score: min(score, 1), Scale: 1}
}

// waterColor is the mean CIELAB colour of the pixels near the median colour of
// area, which is mostly water.
func waterColor(frame *image.RGBA, area image.Rectangle) ([3]float64, bool) {
	var samples [][3]float64
	for y := area.Min.Y; y < area.Max.Y; y += 4 {
		for x := area.Min.X; x < area.Max.X; x += 4 {
			p := frame.RGBAAt(x, y)
			if p.R|p.G|p.B != 0 {
				samples = append(samples, lab(p.R, p.G, p.B))
			}
		}
	}
	if len(samples) == 0 {
		return [3]float64{}, false
	}
	var median [3]float64
	for c := range median {
		values := make([]float64, len(samples))
		for i, s := range samples {
			values[i] = s[c]
		}
		slices.Sort(values)
		median[c] = values[len(values)/2]
	}
	var sum [3]float64
	n := 0
	for _, s := range samples {
		if deltaE(s, median) < colorWaterSpread {
			sum[0], sum[1], sum[2] = sum[0]+s[0], sum[1]+s[1], sum[2]+s[2]
			n++
		}
	}
	if n == 0 {
		return median, true
	}
	return [3]float64{sum[0] / float64(n), sum[1] / float64(n), sum[2] / float64(n)}, true
}

// tanScore rates how much a pixel looks like the bobber's body: bright, with a
// hue near 35 degrees and a saturation near 0.35. Pixels too dark, too bright
// or too grey score zero.
func tanScore(r, g, b int) float64 {
	hi, lo := max(r, g, b), min(r, g, b)
	luma := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
	if luma < 30 || luma > 225 || hi == 0 {
		return 0
	}
	sat := float64(hi-lo) / float64(hi)
	if sat < 0.15 {
		return 0
	}
	var hue float64
	d := float64(hi - lo)
	switch hi {
	case r:
		hue = math.Mod(float64(g-b)/d+6, 6)
	case g:
		hue = float64(b-r)/d + 2
	default:
		hue = float64(r-g)/d + 4
	}
	return 1000*float64(hi)/255 - 10*math.Abs(hue*60-35) - 1000*math.Abs(sat-0.35)
}

// lab converts an sRGB colour to CIELAB (D65).
func lab(r, g, b uint8) [3]float64 {
	lr, lg, lb := linear(r), linear(g), linear(b)
	x := (0.4124*lr + 0.3576*lg + 0.1805*lb) / 0.95047
	y := 0.2126*lr + 0.7152*lg + 0.0722*lb
	z := (0.0193*lr + 0.1192*lg + 0.9505*lb) / 1.08883
	fx, fy, fz := labF(x), labF(y), labF(z)
	return [3]float64{116*fy - 16, 500 * (fx - fy), 200 * (fy - fz)}
}

func linear(c uint8) float64 {
	v := float64(c) / 255
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func labF(t float64) float64 {
	if t > 216.0/24389 {
		return math.Cbrt(t)
	}
	return t*24389/27/116 + 16.0/116
}

func deltaE(a, b [3]float64) float64 {
	return math.Sqrt((a[0]-b[0])*(a[0]-b[0]) + (a[1]-b[1])*(a[1]-b[1]) + (a[2]-b[2])*(a[2]-b[2]))
}

var _ Locator = (*ColorLocator)(nil)
