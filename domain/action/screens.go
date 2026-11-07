package action

import (
	"image"
	"sort"
)

// Screen is one monitor in virtual-desktop coordinates. Number is 1-based:
// the primary monitor is 1, the others follow from left to right.
type Screen struct {
	Number  int             `json:"screen"`
	Bounds  image.Rectangle `json:"-"`
	X       int             `json:"x"`
	Y       int             `json:"y"`
	W       int             `json:"w"`
	H       int             `json:"h"`
	Primary bool            `json:"primary"`
}

// numberScreens orders monitors (primary first, then by X, then Y) and
// assigns their 1-based numbers.
func numberScreens(rects []image.Rectangle, primary int) []Screen {
	out := make([]Screen, len(rects))
	for i, r := range rects {
		out[i] = Screen{Bounds: r, X: r.Min.X, Y: r.Min.Y, W: r.Dx(), H: r.Dy(), Primary: i == primary}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Primary != out[b].Primary {
			return out[a].Primary
		}
		if out[a].X != out[b].X {
			return out[a].X < out[b].X
		}
		return out[a].Y < out[b].Y
	})
	for i := range out {
		out[i].Number = i + 1
	}
	return out
}

// ScreenByNumber returns the monitor with the given 1-based number.
func ScreenByNumber(screens []Screen, n int) (Screen, bool) {
	if n < 1 || n > len(screens) {
		return Screen{}, false
	}
	return screens[n-1], true
}
