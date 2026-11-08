package desktop

import (
	"image"
	"path/filepath"
	"time"
)

// View implements the presenters' view interfaces by updating one State, and
// sends it to the frontend on Flush when it changed. All methods run on the
// UI loop.
type View struct {
	state  State
	sent   State
	seq    int
	frames *Frames
	emit   func(State)
}

// NewView returns a View that stores previews in frames and sends State with emit.
func NewView(frames *Frames, emit func(State)) *View {
	return &View{frames: frames, emit: emit, sent: State{FSM: "-"}}
}

// State returns the current state.
func (v *View) State() State { return v.state }

// Flush sends the state if it changed since the last Flush.
func (v *View) Flush() {
	if v.state == v.sent {
		return
	}
	v.sent = v.state
	v.emit(v.state)
}

// SetStateLabel shows the fishing state name.
func (v *View) SetStateLabel(name string) { v.state.FSM = name }

// SetStatus shows a message in the status bar.
func (v *View) SetStatus(text string) { v.state.Status = text }

// SetRunning shows whether capture runs.
func (v *View) SetRunning(running bool) { v.state.Running = running }

// SetRecording shows the active recording directory, "" when none.
func (v *View) SetRecording(dir string) {
	v.state.Recording, v.state.RecordingName = dir != "", ""
	if dir != "" {
		v.state.RecordingName = filepath.Base(dir)
	}
}

// SetSession shows the session and total capture durations.
func (v *View) SetSession(session, total time.Duration) {
	v.state.SessionSeconds, v.state.TotalSeconds = int(session.Seconds()), int(total.Seconds())
}

// UpdateCapture shows a captured frame.
func (v *View) UpdateCapture(img *image.RGBA) {
	v.frames.Put("capture", img)
	v.seq++
	v.state.Frame = v.seq
}

// UpdateDetection shows the area watched for a bite.
func (v *View) UpdateDetection(img *image.RGBA) {
	v.frames.Put("roi", img)
	v.seq++
	v.state.ROI = v.seq
}

// PreviewReset clears the previews.
func (v *View) PreviewReset() {
	v.frames.Clear()
	v.state.Frame, v.state.ROI, v.state.Locked = 0, 0, false
}

// SetTarget shows the locked bobber in frame pixels, or none.
func (v *View) SetTarget(p Point, locked bool) {
	if !locked {
		p = Point{}
	}
	v.state.Target, v.state.Locked = p, locked
}

// SetWindow shows the target window title.
func (v *View) SetWindow(title string) { v.state.Window = title }

// SetSelection shows the capture area and whether the overlay is open.
func (v *View) SetSelection(r Rect, selecting bool) {
	v.state.Selection, v.state.Selecting = r, selecting
}

// SetInfo shows fixed run facts and the theme.
func (v *View) SetInfo(dryRun bool, stopKey string, dark bool) {
	v.state.DryRun, v.state.StopKey, v.state.Dark = dryRun, stopKey, dark
}
