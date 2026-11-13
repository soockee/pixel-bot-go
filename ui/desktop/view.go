package desktop

import (
	"image"
	"path/filepath"
	"time"

	"github.com/soocke/pixel-bot-go/config"
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
func (v *View) State() State {
	s := v.state
	s.Setup, s.SetupDone = setupSteps(s)
	return s
}

// Flush sends the state if it changed since the last Flush.
func (v *View) Flush() {
	s := v.State()
	if s == v.sent {
		return
	}
	v.sent = s
	v.emit(s)
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

// SetOutcomes shows how the run's casts ended, as Bobber Marker reports them.
func (v *View) SetOutcomes(caught, gotAway, notHooked int) {
	v.state.Caught, v.state.GotAway, v.state.NotHooked = caught, gotAway, notHooked
}

// SetRun shows the run's counts and the limits that stop it.
func (v *View) SetRun(casts, reels, attempts, inputs, failedInRow, maxFailed, sessionLimit int) {
	v.state.Casts, v.state.Reels, v.state.FailedInRow = casts, reels, failedInRow
	v.state.ReelAttempts, v.state.ReelInputs = attempts, inputs
	v.state.MaxFailed, v.state.SessionLimit = maxFailed, sessionLimit
}

// UpdateCapture shows a capture of the game window.
func (v *View) UpdateCapture(img *image.RGBA) {
	v.frames.Put("capture", shrink(img, previewWidth))
	v.seq++
	size := img.Bounds().Size()
	v.state.Frame, v.state.FrameSize = v.seq, Point{X: size.X, Y: size.Y}
}

// UpdateDetection shows the live box watched for a bite and the live area
// around it, each at its box on the capture, or none when its image is nil.
func (v *View) UpdateDetection(roi *image.RGBA, roiBox image.Rectangle, scene *image.RGBA, sceneBox image.Rectangle) {
	v.state.ROI, v.state.ROIBox = v.put("roi", roi, roiBox)
	v.state.Scene, v.state.SceneBox = v.put("scene", scene, sceneBox)
}

// put stores img as the newest image of name and returns its version and box,
// or zeros when img is nil.
func (v *View) put(name string, img *image.RGBA, box image.Rectangle) (int, Rect) {
	if img == nil {
		return 0, Rect{}
	}
	v.frames.Put(name, img)
	v.seq++
	return v.seq, Rect{X: box.Min.X, Y: box.Min.Y, W: box.Dx(), H: box.Dy()}
}

// PreviewReset clears the previews.
func (v *View) PreviewReset() {
	v.frames.Clear()
	v.state.Frame, v.state.ROI, v.state.ROIBox, v.state.Scene, v.state.SceneBox = 0, 0, Rect{}, 0, Rect{}
}

// SetWindow shows the target window title.
func (v *View) SetWindow(title string) { v.state.Window = title }

// SetGameFound shows whether the selected window is open.
func (v *View) SetGameFound(found bool) { v.state.GameFound = found }

// SetKeysChosen marks the game keys setup step as done.
func (v *View) SetKeysChosen() { v.state.KeysChosen = true }

// SetInput shows the saved board port and the keys the next run uses.
func (v *View) SetInput(c *config.Config) {
	v.state.ESP32Port, v.state.Keys = c.ESP32Port, setupKeys(c)
}

// SetBoardFound shows the port setup found the board on.
func (v *View) SetBoardFound(port string) { v.state.BoardFound = port }

// SetUpdate shows the newer release on offer, "" when none, and whether it
// is being installed.
func (v *View) SetUpdate(tag string, updating bool) { v.state.Update, v.state.Updating = tag, updating }

// SetChecking shows that a release check runs; SetUpdateCheck ends it.
func (v *View) SetChecking() { v.state.Checking = true }

// SetUpdateCheck shows the latest release found and when the app asked,
// with the reason if the check failed.
func (v *View) SetUpdateCheck(latest string, released, checked time.Time, problem string) {
	v.state.Checking, v.state.Latest, v.state.UpdateError = false, latest, problem
	v.state.LatestReleased, v.state.UpdateChecked = unix(released), unix(checked)
}

// unix returns t in Unix seconds, 0 for the zero time.
func unix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// SetAddon shows where the addon goes and its status there; see State.Addon.
func (v *View) SetAddon(dir, status string) { v.state.AddonDir, v.state.Addon = dir, status }

// SetAddonVersion shows the version of the addon the app carries.
func (v *View) SetAddonVersion(version string) { v.state.AddonVersion = version }

// SetOverlay shows where the overlay pill sits, and whether it is being moved.
func (v *View) SetOverlay(x, y float64, moving bool) {
	v.state.OverlayX, v.state.OverlayY, v.state.OverlayMove = x, y, moving
}

// SetVersion shows the running build's version.
func (v *View) SetVersion(version string) { v.state.Version = version }

// SetInfo shows fixed run facts.
func (v *View) SetInfo(dryRun, debug bool) {
	v.state.DryRun, v.state.Debug = dryRun, debug
}

// SetTheme shows the window's look.
func (v *View) SetTheme(theme string) { v.state.Theme = theme }
