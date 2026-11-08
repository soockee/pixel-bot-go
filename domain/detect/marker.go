package detect

import (
	"image"
	"image/color"
)

// markerColor is the colour of the addon's brackets around the player's bobber
// over a bright scene. They are partly transparent, so a dark scene darkens green
// and blue, down to about (255, 144, 141) at night.
var markerColor = color.RGBA{255, 178, 163, 255}

const (
	markerMinArm    = 10  // shorter strokes are not a bracket
	markerMinSide   = 30  // pixels
	markerMaxSide   = 400 // pixels
	markerSlack     = 3   // how far a corner may sit from the box corner
	markerThickness = 3
	markerArm       = 19
)

// corner is an L-shaped bracket. Its arms run from (x, y) along dx and dy.
type corner struct{ x, y, dx, dy int }

// MarkerLocator finds the corner brackets an addon draws around the player's own
// bobber, so other players' bobbers are never considered. The brackets give both
// the click point and the area to watch for a bite. It reuses a buffer between
// calls, so call Locate from one goroutine.
type MarkerLocator struct {
	mask []bool
	w, h int
}

// NewMarkerLocator returns a MarkerLocator.
func NewMarkerLocator() *MarkerLocator { return &MarkerLocator{} }

// Locate finds the box whose brackets show at least three corners, because
// another effect can cover one. Score is the share of corners seen.
func (l *MarkerLocator) Locate(frame *image.RGBA) (Match, bool) {
	l.load(frame)
	corners := l.corners()
	var box image.Rectangle
	seen := 0
	for i, a := range corners {
		for _, b := range corners[i+1:] {
			if a.dx != -b.dx || a.dy != -b.dy {
				continue // only opposite corners define a box
			}
			candidate := image.Rect(a.x, a.y, b.x, b.y)
			candidate.Max = candidate.Max.Add(image.Pt(1, 1))
			if !validSide(candidate.Dx()) || !validSide(candidate.Dy()) {
				continue
			}
			if n := cornersOn(candidate, corners); n > seen {
				box, seen = candidate, n
			}
		}
	}
	match := Match{Score: float64(seen) / 4, Scale: 1}
	if seen < 3 {
		return match, false
	}
	match.X, match.Y = (box.Min.X+box.Max.X)/2, (box.Min.Y+box.Max.Y)/2
	match.Box = box
	return match, true
}

func validSide(n int) bool { return n >= markerMinSide && n <= markerMaxSide }

// cornersOn counts the box corners that have a matching bracket.
func cornersOn(box image.Rectangle, corners []corner) int {
	n := 0
	for _, dx := range []int{1, -1} {
		for _, dy := range []int{1, -1} {
			x, y := box.Min.X, box.Min.Y
			if dx < 0 {
				x = box.Max.X - 1
			}
			if dy < 0 {
				y = box.Max.Y - 1
			}
			for _, c := range corners {
				if c.dx == dx && c.dy == dy && abs(c.x-x) <= markerSlack && abs(c.y-y) <= markerSlack {
					n++
					break
				}
			}
		}
	}
	return n
}

func abs(v int) int { return max(v, -v) }

// load marks the pixels that have the marker colour.
func (l *MarkerLocator) load(frame *image.RGBA) {
	b := frame.Bounds()
	l.w, l.h = b.Dx(), b.Dy()
	if need := l.w * l.h; cap(l.mask) < need {
		l.mask = make([]bool, need)
	} else {
		l.mask = l.mask[:need]
	}
	for y := range l.h {
		pix := frame.Pix[frame.PixOffset(b.Min.X, b.Min.Y+y):]
		for x := range l.w {
			l.mask[y*l.w+x] = isMarkerColor(pix[x*4], pix[x*4+1], pix[x*4+2])
		}
	}
}

// isMarkerColor accepts the brackets over any scene: red near full, green and
// blue well below it and close together. Gold and orange have far more green
// than blue, so the bobber's feather and similar effects fail.
func isMarkerColor(r, g, b uint8) bool {
	return r >= 230 && int(r)-int(g) >= 50 && abs(int(g)-int(b)) <= 35
}

func (l *MarkerLocator) at(x, y int) bool {
	return x >= 0 && y >= 0 && x < l.w && y < l.h && l.mask[y*l.w+x]
}

// corners finds every outer bracket corner: a marked pixel with two long arms
// and nothing behind it. A star's crossing arms fail the last check.
func (l *MarkerLocator) corners() []corner {
	var found []corner
	for y := range l.h {
		for x := range l.w {
			if !l.mask[y*l.w+x] {
				continue
			}
			for _, dx := range []int{1, -1} {
				for _, dy := range []int{1, -1} {
					if !l.at(x-dx, y) && !l.at(x, y-dy) && l.arm(x, y, dx, 0) && l.arm(x, y, 0, dy) {
						found = append(found, corner{x, y, dx, dy})
					}
				}
			}
		}
	}
	return found
}

func (l *MarkerLocator) arm(x, y, dx, dy int) bool {
	for i := range markerMinArm {
		if !l.at(x+i*dx, y+i*dy) {
			return false
		}
	}
	return true
}

// DrawMarker draws the addon's brackets around box, for the simulator and tests.
func DrawMarker(img *image.RGBA, box image.Rectangle) {
	arm := min(markerArm, box.Dx()/3, box.Dy()/3)
	fill := func(r image.Rectangle) {
		r = r.Intersect(img.Bounds())
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				img.SetRGBA(x, y, markerColor)
			}
		}
	}
	for _, x := range []int{box.Min.X, box.Max.X - markerThickness} {
		fill(image.Rect(x, box.Min.Y, x+markerThickness, box.Min.Y+arm))
		fill(image.Rect(x, box.Max.Y-arm, x+markerThickness, box.Max.Y))
	}
	for _, y := range []int{box.Min.Y, box.Max.Y - markerThickness} {
		fill(image.Rect(box.Min.X, y, box.Min.X+arm, y+markerThickness))
		fill(image.Rect(box.Max.X-arm, y, box.Max.X, y+markerThickness))
	}
}
