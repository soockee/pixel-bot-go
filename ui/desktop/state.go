// Package desktop connects the presenters to the desktop window. It has no
// window toolkit dependency: View turns presenter calls into one State value,
// Service turns frontend calls into commands, and Loop runs both on one
// goroutine. The frontend only renders State and calls Service.
package desktop

import "image"

// StateEvent is the event that carries State to the frontend.
const StateEvent = "state"

// Rect is a screen rectangle in physical pixels. A zero width means none.
type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// RectOf returns r as a Rect, or the zero Rect for nil.
func RectOf(r *image.Rectangle) Rect {
	if r == nil {
		return Rect{}
	}
	return Rect{X: r.Min.X, Y: r.Min.Y, W: r.Dx(), H: r.Dy()}
}

// Point is a position in pixels.
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// State is everything the main window shows.
type State struct {
	FSM            string `json:"fsm"` // fishing state name, see fishing.FishingState.String
	Running        bool   `json:"running"`
	Recording      bool   `json:"recording"`
	RecordingName  string `json:"recordingName"` // directory name of the active recording, or ""
	Status         string `json:"status"`
	Window         string `json:"window"`
	SessionSeconds int    `json:"sessionSeconds"`
	TotalSeconds   int    `json:"totalSeconds"`
	Frame          int    `json:"frame"`  // version of /frames/capture; 0 means none
	ROI            int    `json:"roi"`    // version of /frames/roi; 0 means none
	Locked         bool   `json:"locked"` // Target holds the locked bobber
	Target         Point  `json:"target"` // in capture frame pixels
	Selection      Rect   `json:"selection"`
	Selecting      bool   `json:"selecting"` // the selection overlay is open
	DryRun         bool   `json:"dryRun"`
	StopKey        string `json:"stopKey"`
	Dark           bool   `json:"dark"`
}
