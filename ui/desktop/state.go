// Package desktop connects the presenters to the desktop window. It has no
// window toolkit dependency: View turns presenter calls into one State value,
// Service turns frontend calls into commands, and Loop runs both on one
// goroutine. The frontend only renders State and calls Service.
package desktop

// StateEvent is the event that carries State to the frontend.
const StateEvent = "state"

// Rect is a rectangle in screen pixels. A zero width means none.
type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// Point is a position in pixels.
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// State is everything the main window shows.
type State struct {
	FSM            string                `json:"fsm"` // fishing state name, see fishing.FishingState.String
	Running        bool                  `json:"running"`
	Recording      bool                  `json:"recording"`
	RecordingName  string                `json:"recordingName"` // directory name of the active recording, or ""
	Status         string                `json:"status"`
	Window         string                `json:"window"`
	SessionSeconds int                   `json:"sessionSeconds"`
	SessionLimit   int                   `json:"sessionLimit"` // minutes; the run stops at the first cast after it
	Casts          int                   `json:"casts"`        // this run's casts
	Reels          int                   `json:"reels"`        // this run's reels, each a bite answered
	FailedInRow    int                   `json:"failedInRow"`  // casts in a row without a reel
	MaxFailed      int                   `json:"maxFailed"`    // the run stops when FailedInRow reaches it
	Frame          int                   `json:"frame"`        // version of /frames/capture; 0 means none
	FrameSize      Point                 `json:"frameSize"`    // the capture's size in screen pixels; the image may be smaller
	ROI            int                   `json:"roi"`          // version of /frames/roi, the live box around the bobber; 0 means none
	ROIBox         Rect                  `json:"roiBox"`       // where the ROI sits on the capture, in screen pixels
	DryRun         bool                  `json:"dryRun"`
	StopKey        string                `json:"stopKey"`
	Dark           bool                  `json:"dark"`
	Debug          bool                  `json:"debug"`          // show the developer views
	Recorder       bool                  `json:"recorder"`       // show the Recorder: debug mode or Share recordings
	SignIn         bool                  `json:"signIn"`         // sign-in is configured; without it fishing needs no role
	Account        string                `json:"account"`        // signed-in Google email, or ""
	Role           string                `json:"role"`           // the account's role, "" when nobody gave it one
	CanFish        bool                  `json:"canFish"`        // the role may start a run (dry runs need no role)
	CanUpload      bool                  `json:"canUpload"`      // the role may upload recordings
	Uploading      string                `json:"uploading"`      // recording being uploaded, or ""
	Update         string                `json:"update"`         // newer release tag on offer, or ""
	Updating       bool                  `json:"updating"`       // the update downloads or installs
	Latest         string                `json:"latest"`         // latest release tag found, "" before the first check
	LatestReleased int64                 `json:"latestReleased"` // when Latest was published, Unix seconds
	UpdateChecked  int64                 `json:"updateChecked"`  // when the app last asked for releases, Unix seconds; 0 never
	UpdateError    string                `json:"updateError"`    // why the last check failed, or why there is none
	Checking       bool                  `json:"checking"`       // a check runs
	GameFound      bool                  `json:"gameFound"`      // the selected window is open
	Setup          [setupSlots]SetupStep `json:"setup"`          // what the bot still needs, see setupSteps
	SetupDone      bool                  `json:"setupDone"`      // nothing is missing, or this is a dry run
	Addon          string                `json:"addon"`          // Bobber Marker in the game: "missing", "outdated", "current", or "" when the game was not found
	AddonDir       string                `json:"addonDir"`       // where the addon is or goes
	AddonVersion   string                `json:"addonVersion"`   // the version the app carries
	OverlayX       float64               `json:"overlayX"`       // where the overlay pill sits across the game window: 0 is the left edge, 1 the right
	OverlayY       float64               `json:"overlayY"`       // where the overlay pill sits down the game window: 0 is the top edge, 1 the bottom
	OverlayMove    bool                  `json:"overlayMove"`    // the overlay is being placed: it takes the mouse and shows only the pill
	Version        string                `json:"version"`        // the running build's release tag, "dev" for local builds
}
