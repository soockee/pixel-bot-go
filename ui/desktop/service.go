package desktop

import (
	"errors"
	"log/slog"
	"slices"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/detect"
	"github.com/soocke/pixel-bot-go/internal/esp32"
	"github.com/soocke/pixel-bot-go/internal/recorder"
)

// Commands are the app actions the window can start. They run on the UI loop.
type Commands struct {
	ListWindows   func() []string
	ListPorts     func() []string // COM ports for the ESP32 keyboard
	SetWindow     func(title string)
	ToggleCapture func()
	ToggleRecord  func()
	Running       func() bool
	Exit          func()
	// Update runs on the caller's goroutine and waits for the network.
	Update func() error
	// CheckUpdate asks GitHub for the latest release; it also waits for the network.
	CheckUpdate  func() error
	ReleaseNotes func()
	// ReportProblem opens a new GitHub issue, filled in, in the browser.
	ReportProblem func()
	// OpenURL opens a page in the browser.
	OpenURL      func(url string)
	CheckAddon   func()
	InstallAddon func() error
	// SetTheme shows the theme's icon on the window and the taskbar.
	SetTheme func(theme string)
	// MoveOverlay starts or ends placing the overlay; ending saves its place.
	MoveOverlay func(on bool) error
	// FindBoard asks for the Go Fish firmware on port, or on every USB serial
	// port for esp32.AutoPort, and returns the port that answered. It runs on
	// the caller's goroutine, as the handshake takes a while, and sends no input.
	FindBoard func(port string) (string, error)
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

// Windows lists the visible window titles, including the selected title even
// when it is closed. The game is the default until the person picks a window.
func (s *Service) Windows() []string {
	titles := s.cmds.ListWindows()
	_ = s.do(func() {
		current := pickWindow(s.view.State().Window, s.picked)
		if !slices.Contains(titles, current) {
			titles = append(titles, current)
		}
		if current != s.view.State().Window && !s.cmds.Running() {
			s.cmds.SetWindow(current)
		}
	})
	return titles
}

// ESP32Ports lists the COM ports to offer for the ESP32 keyboard. The saved
// port stays in the list while its board is unplugged.
func (s *Service) ESP32Ports() []string {
	ports := s.cmds.ListPorts()
	_ = s.do(func() {
		if s.cfg.ESP32Port != "" && !slices.Contains(ports, s.cfg.ESP32Port) {
			ports = append(ports, s.cfg.ESP32Port)
		}
	})
	return ports
}

// SetWindow makes title the target window.
func (s *Service) SetWindow(title string) {
	_ = s.do(func() {
		s.picked = true
		s.cmds.SetWindow(title)
	})
}

// CheckGame looks for the game window, for the setup steps. Unless the person
// chose a window, it selects the game as soon as it opens, so starting the game
// after Go Fish needs no click. The frontend calls it now and then.
func (s *Service) CheckGame() {
	titles := s.cmds.ListWindows()
	_ = s.do(func() {
		current := s.view.State().Window
		if !s.picked && !s.cmds.Running() && current != defaultWindow {
			s.cmds.SetWindow(defaultWindow)
			current = defaultWindow
		}
		s.view.SetGameFound(slices.Contains(titles, current))
		if s.cmds.CheckAddon != nil {
			s.cmds.CheckAddon() // the game's folder comes from its window
		}
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
		if err = next.Save(s.cfgPath); err != nil {
			s.logger.Error("config save failed", "error", err)
			return
		}
		*s.cfg = next
		s.view.SetInput(s.cfg)
		if s.cmds.CheckAddon != nil {
			s.cmds.CheckAddon() // the addon binds the Cast and Interact keys, so a change makes it outdated
		}
		s.logger.Info("config saved", "path", s.cfgPath)
		s.view.SetStatus("Settings saved")
		saved = settingsOf(s.cfg)
	}); doErr != nil {
		return saved, doErr
	}
	return saved, err
}

// SetTheme saves the window's look, one of config.Themes.
func (s *Service) SetTheme(theme string) {
	if !slices.Contains(config.Themes, theme) {
		return
	}
	_ = s.do(func() {
		s.cfg.Theme = theme
		s.view.SetTheme(theme)
		s.cmds.SetTheme(theme)
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

// Update downloads, verifies and installs the newer release, then restarts.
func (s *Service) Update() error { return s.cmds.Update() }

// CheckUpdate looks for a newer release now; the result arrives in State.
func (s *Service) CheckUpdate() error { return s.cmds.CheckUpdate() }

// ReleaseNotes opens the latest release's page in the browser.
func (s *Service) ReleaseNotes() { _ = s.do(s.cmds.ReleaseNotes) }

// ReportProblem opens a new GitHub issue on the problem form in the browser,
// with the version, Windows and app details filled in.
func (s *Service) ReportProblem() { _ = s.do(s.cmds.ReportProblem) }

// ChooseKeys confirms the saved game keys in setup, so the addon step comes next.
func (s *Service) ChooseKeys() { _ = s.do(s.view.SetKeysChosen) }

// FindBoard looks for the ESP32 board on the saved port, or on every USB
// serial port when none is saved, and returns the port it answered on. The
// frontend then saves the choice through SaveSettings. A run holds the port,
// so it refuses during one.
func (s *Service) FindBoard() (found string, err error) {
	port := esp32.AutoPort
	if doErr := s.do(func() {
		if s.cmds.Running() {
			err = errors.New("stop the run to look for the board")
		} else if s.cfg.ESP32Port != "" {
			port = s.cfg.ESP32Port
		}
	}); doErr != nil {
		return "", doErr
	}
	if err != nil {
		return "", err
	}
	if found, err = s.cmds.FindBoard(port); err != nil {
		return "", err
	}
	_ = s.do(func() { s.view.SetBoardFound(found) })
	return found, nil
}

// CheckAddon looks whether the Bobber Marker addon is installed and current.
func (s *Service) CheckAddon() { _ = s.do(s.cmds.CheckAddon) }

// InstallAddon installs or updates the Bobber Marker addon in the game.
func (s *Service) InstallAddon() (err error) {
	if doErr := s.do(func() { err = s.cmds.InstallAddon() }); doErr != nil {
		return doErr
	}
	return err
}

// MoveOverlay starts placing the overlay, where the person drags it over the
// game, or ends it and keeps the place. A run in progress refuses it.
func (s *Service) MoveOverlay(on bool) (err error) {
	if doErr := s.do(func() {
		if on && s.cmds.Running() {
			err = errors.New("stop the run to place the overlay")
			return
		}
		err = s.cmds.MoveOverlay(on)
	}); doErr != nil {
		return doErr
	}
	return err
}

// Exit closes the app.
func (s *Service) Exit() { _ = s.do(s.cmds.Exit) }

func newSettingsForm(c *config.Config) SettingsForm {
	return SettingsForm{Values: settingsOf(c), Locators: detect.LocatorNames(), BiteDetectors: detect.BiteDetectorNames()}
}

// defaultWindow is selected until the person picks another.
const defaultWindow = "World of Warcraft"

// pickWindow returns the window to select. A window the person picked stays
// selected even if it is not open yet.
func pickWindow(current string, picked bool) string {
	if picked {
		return current
	}
	return defaultWindow
}

// OpenSupport opens the coffee page in the browser and hides the thank-you card.
func (s *Service) OpenSupport() {
	_ = s.do(func() {
		s.cmds.OpenURL(SupportURL)
		s.view.SetSupport(Support{})
	})
}

// DismissSupport hides the thank-you card until the next milestone ("Not now").
func (s *Service) DismissSupport() { _ = s.do(func() { s.view.SetSupport(Support{}) }) }

// MuteSupport hides the thank-you card for good: the person supports Go Fish
// already or does not want to be asked. The status bar link stays.
func (s *Service) MuteSupport() {
	_ = s.do(func() {
		s.view.SetSupport(Support{})
		s.cfg.SupportMuted = true
		if err := s.cfg.Save(s.cfgPath); err != nil {
			s.logger.Error("persist support choice failed", "error", err)
		}
	})
}
