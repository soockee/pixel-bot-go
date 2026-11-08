package desktop

import (
	"errors"
	"log/slog"
	"slices"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/detect"
	"github.com/soocke/pixel-bot-go/internal/recorder"
)

// Commands are the app actions the window can start. They run on the UI loop.
type Commands struct {
	ListWindows   func() []string
	SetWindow     func(title string)
	ToggleCapture func()
	ToggleRecord  func()
	Running       func() bool
	Exit          func()
	// SignIn, SignOut, Upload and Update run on the caller's goroutine:
	// sign-in waits for the browser, an upload and an update for the network.
	SignIn  func() error
	SignOut func()
	Upload  func(dir string) error
	Update  func() error
	// CheckUpdate asks GitHub for the latest release; it also waits for the network.
	CheckUpdate  func() error
	ReleaseNotes func()
	CheckAddon   func()
	InstallAddon func() error
}

// Service is what the frontend calls. Every method runs on the UI loop, so
// it sees the same state as the presenters.
type Service struct {
	loop       *Loop
	view       *View
	cmds       Commands
	recordings *Recordings
	cfg        *config.Config
	cfgPath    string
	logger     *slog.Logger
	picked     bool // the person chose a window, so a refresh keeps it
}

// NewService returns the frontend's service.
func NewService(loop *Loop, view *View, cmds Commands, recordings *Recordings, cfg *config.Config, cfgPath string, logger *slog.Logger) *Service {
	return &Service{loop: loop, view: view, cmds: cmds, recordings: recordings, cfg: cfg, cfgPath: cfgPath, logger: logger,
		picked: cfg.AutoWindow != ""}
}

var errStopped = errors.New("the app is closing")

// do runs f on the UI loop.
func (s *Service) do(f func()) error {
	if !s.loop.Do(f) {
		return errStopped
	}
	return nil
}

// State returns the current state; later changes arrive as StateEvent.
func (s *Service) State() (state State) {
	_ = s.do(func() { state = s.view.State() })
	return state
}

// Windows lists the visible window titles and picks the target window: the
// one the person chose, else the game, else the first one. A chosen window
// stays in the list even when it is not open.
func (s *Service) Windows() []string {
	titles := s.cmds.ListWindows()
	if len(titles) == 0 {
		titles = []string{"<none>"}
	}
	_ = s.do(func() {
		current := pickWindow(titles, s.view.State().Window, s.picked)
		if !slices.Contains(titles, current) {
			titles = append(titles, current)
		}
		s.cmds.SetWindow(current)
	})
	return titles
}

// SetWindow makes title the target window.
func (s *Service) SetWindow(title string) {
	_ = s.do(func() {
		s.picked = true
		s.cmds.SetWindow(title)
	})
}

// ToggleCapture starts or stops the run.
func (s *Service) ToggleCapture() { _ = s.do(s.cmds.ToggleCapture) }

// ToggleRecord starts or stops a recording.
func (s *Service) ToggleRecord() { _ = s.do(s.cmds.ToggleRecord) }

// Settings returns the settings form.
func (s *Service) Settings() (form SettingsForm) {
	_ = s.do(func() { form = newSettingsForm(s.cfg) })
	return form
}

// SaveSettings validates and saves settings, and returns the values as saved,
// clamped to safe ranges. Settings cannot change during a run.
func (s *Service) SaveSettings(values Settings) (saved Settings, err error) {
	if doErr := s.do(func() {
		if s.cmds.Running() {
			err = errors.New("stop the run to change settings")
			return
		}
		var next config.Config
		if next, err = values.apply(*s.cfg); err != nil {
			return
		}
		if next.CastKey != s.cfg.CastKey && sameKey(next.CastKey, s.view.State().StopKey) {
			err = errors.New("the Cast key matches the active Stop hotkey")
			return
		}
		if next.ReelMethod == config.ReelInteractKey && sameKey(next.InteractKey, s.view.State().StopKey) {
			err = errors.New("the Interact key matches the active Stop hotkey")
			return
		}
		if next.Schedule && sameKey(next.LogoutKey, s.view.State().StopKey) {
			err = errors.New("the Logout key matches the active Stop hotkey")
			return
		}
		if err = next.Save(s.cfgPath); err != nil {
			s.logger.Error("config save failed", "error", err)
			return
		}
		*s.cfg = next
		s.view.SetRecorder(next.RecorderShown())
		s.logger.Info("config saved", "path", s.cfgPath)
		s.view.SetStatus("Settings saved")
		saved = settingsOf(s.cfg)
	}); doErr != nil {
		return saved, doErr
	}
	return saved, err
}

// SetDark saves the theme choice.
func (s *Service) SetDark(dark bool) {
	_ = s.do(func() {
		s.cfg.DarkMode = dark
		st := s.view.State()
		s.view.SetInfo(st.DryRun, st.Debug, st.StopKey, dark)
		if err := s.cfg.Save(s.cfgPath); err != nil {
			s.logger.Error("persist theme failed", "error", err)
		}
	})
}

// Recordings lists the saved recordings, newest first.
func (s *Service) Recordings() ([]Recording, error) { return s.recordings.List() }

// RecordingEvents reads the input trace of recording name from offset on.
func (s *Service) RecordingEvents(name string, offset int64) (recorder.EventPage, error) {
	return s.recordings.Events(name, offset)
}

// SignIn signs in with Google in the browser. It returns once signed in.
func (s *Service) SignIn() error { return s.cmds.SignIn() }

// SignOut forgets the signed-in account.
func (s *Service) SignOut() { s.cmds.SignOut() }

// Upload sends recording name to the upload service and returns when done.
func (s *Service) Upload(name string) error {
	dir, err := s.recordings.path(name)
	if err != nil {
		return err
	}
	return s.cmds.Upload(dir)
}

// Update downloads, verifies and installs the newer release, then restarts.
func (s *Service) Update() error { return s.cmds.Update() }

// CheckUpdate looks for a newer release now; the result arrives in State.
func (s *Service) CheckUpdate() error { return s.cmds.CheckUpdate() }

// ReleaseNotes opens the latest release's page in the browser.
func (s *Service) ReleaseNotes() { _ = s.do(s.cmds.ReleaseNotes) }

// CheckAddon looks whether the Bobber Marker addon is installed and current.
func (s *Service) CheckAddon() { _ = s.do(s.cmds.CheckAddon) }

// InstallAddon installs or updates the Bobber Marker addon in the game.
func (s *Service) InstallAddon() (err error) {
	if doErr := s.do(func() { err = s.cmds.InstallAddon() }); doErr != nil {
		return doErr
	}
	return err
}

// Exit closes the app.
func (s *Service) Exit() { _ = s.do(s.cmds.Exit) }

func newSettingsForm(c *config.Config) SettingsForm {
	return SettingsForm{Values: settingsOf(c), Locators: detect.LocatorNames(), BiteDetectors: detect.BiteDetectorNames()}
}

// defaultWindow is selected whenever it is open, until the person picks another.
const defaultWindow = "World of Warcraft"

// pickWindow returns the window to select. A window the person picked stays
// selected even if it is not open yet. titles is not empty.
func pickWindow(titles []string, current string, picked bool) string {
	switch {
	case picked:
		return current
	case slices.Contains(titles, defaultWindow):
		return defaultWindow
	}
	return titles[0]
}
