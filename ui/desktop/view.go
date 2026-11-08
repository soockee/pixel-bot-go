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

// SetSession shows the session duration; the window has no use for the total.
func (v *View) SetSession(session, _ time.Duration) { v.state.SessionSeconds = int(session.Seconds()) }

// SetRun shows the run's counts and the limits that stop it.
func (v *View) SetRun(casts, reels, failedInRow, maxFailed, sessionLimit int) {
	v.state.Casts, v.state.Reels, v.state.FailedInRow = casts, reels, failedInRow
	v.state.MaxFailed, v.state.SessionLimit = maxFailed, sessionLimit
}

// UpdateCapture shows a capture of the game window.
func (v *View) UpdateCapture(img *image.RGBA) {
	v.frames.Put("capture", shrink(img, previewWidth))
	v.seq++
	size := img.Bounds().Size()
	v.state.Frame, v.state.FrameSize = v.seq, Point{X: size.X, Y: size.Y}
}

// UpdateDetection shows the live box watched for a bite, at box on the
// capture, or none when img is nil.
func (v *View) UpdateDetection(img *image.RGBA, box image.Rectangle) {
	if img == nil {
		v.state.ROI, v.state.ROIBox = 0, Rect{}
		return
	}
	v.frames.Put("roi", img)
	v.seq++
	v.state.ROI, v.state.ROIBox = v.seq, Rect{X: box.Min.X, Y: box.Min.Y, W: box.Dx(), H: box.Dy()}
}

// PreviewReset clears the previews.
func (v *View) PreviewReset() {
	v.frames.Clear()
	v.state.Frame, v.state.ROI, v.state.ROIBox = 0, 0, Rect{}
}

// SetWindow shows the target window title.
func (v *View) SetWindow(title string) { v.state.Window = title }

// SetAccount shows the signed-in account and what its role may do; an empty
// email means signed out.
func (v *View) SetAccount(email, role string, canFish, canUpload bool) {
	v.state.Account, v.state.Role, v.state.CanFish, v.state.CanUpload = email, role, canFish, canUpload
}

// SetUploading shows the recording being uploaded, "" when none.
func (v *View) SetUploading(name string) { v.state.Uploading = name }

// SetUpdate shows the newer release on offer, "" when none, and whether it
// is being installed.
func (v *View) SetUpdate(tag string, updating bool) { v.state.Update, v.state.Updating = tag, updating }

// SetVersion shows the running build's version.
func (v *View) SetVersion(version string) { v.state.Version = version }

// SetInfo shows fixed run facts and the theme.
func (v *View) SetInfo(dryRun, debug bool, stopKey string, dark bool) {
	v.state.DryRun, v.state.Debug, v.state.StopKey, v.state.Dark = dryRun, debug, stopKey, dark
}
