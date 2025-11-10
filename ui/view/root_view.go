package view

import (
	"image"
	"log/slog"
	"strconv"

	"github.com/soocke/pixel-bot-go/config"

	//lint:ignore ST1001 Dot import is intentional for concise Tk widget DSL builders.
	. "modernc.org/tk9.0"
)

// RootView composes the top-level application layout and wires UI callbacks.
// It owns high-level subviews but exposes minimal exported fields for presenters.
type RootView struct {
	cfg     *config.Config
	cfgPath string
	logger  *slog.Logger

	// Subviews
	Session     SessionStats
	ConfigPanel ConfigPanel
	CapturePrev CapturePreview

	// Widgets
	StateLabel   *LabelWidget
	WindowSelect *TComboboxWidget
	captureRow   int
}

// UI abstracts the subset of view operations needed by presenters, enabling decoupling
// from the concrete RootView implementation.
type UI interface {
	SetStateLabel(text string)
	SetConfigEditable(enabled bool)
	UpdateCapture(img image.Image)
	UpdateDetection(img image.Image)
	SetSession(seconds int, totalSeconds int)
}

func NewRootView(cfg *config.Config, cfgPath string, logger *slog.Logger) *RootView {
	return &RootView{cfg: cfg, cfgPath: cfgPath, logger: logger}
}

// Build constructs the layout. titles: list of window titles for selection dropdown.
// Handlers are invoked on user actions.
func (rv *RootView) Build(titles []string, onToggleCapture func(), onSelectionGrid func(), onExit func(), onWindowChanged func(title string)) {
	if rv == nil {
		return
	}
	// Row 0: session stats, state label, buttons frame
	rv.Session = NewSessionStats(0)
	rv.StateLabel = Label(Txt("State: <none>"), Borderwidth(1), Relief("ridge"))
	Grid(rv.StateLabel, Row(0), Column(2), Sticky("we"), Padx("0.4m"), Pady("0.3m"))

	btnFrame := Frame()
	Grid(btnFrame, Row(0), Column(4), Sticky("ne"), Padx("0.3m"), Pady("0.3m"))
	captureBtn := Button(Txt("Toggle Capture"), Command(onToggleCapture))
	Grid(captureBtn, In(btnFrame), Row(0), Column(0), Sticky("we"), Padx("0.2m"), Pady("0.2m"))
	if len(titles) == 0 {
		titles = []string{"<none>"}
	}
	rv.WindowSelect = TCombobox(Values(titles), Width(26))
	Grid(rv.WindowSelect, In(btnFrame), Row(1), Column(0), Sticky("we"), Padx("0.2m"), Pady("0.2m"))
	rv.WindowSelect.Current(0)
	Bind(rv.WindowSelect, "<<ComboboxSelected>>", Command(func() {
		if rv.WindowSelect != nil {
			idxStr := rv.WindowSelect.Current(nil)
			idx, err := strconv.Atoi(idxStr)
			if err == nil && idx >= 0 && idx < len(titles) {
				onWindowChanged(titles[idx])
			} else {
				if rv.logger != nil {
					rv.logger.Error("window selection parse error", "error", err)
				}
			}
		}
	}))
	selectionBtn := Button(Txt("Selection Grid"), Command(onSelectionGrid))
	Grid(selectionBtn, In(btnFrame), Row(2), Column(0), Sticky("we"), Padx("0.2m"), Pady("0.2m"))
	exitBtn := Button(Txt("Exit"), Command(onExit))
	Grid(exitBtn, In(btnFrame), Row(3), Column(0), Sticky("we"), Padx("0.2m"), Pady("0.2m"))

	// Config panel rows
	rv.ConfigPanel = NewConfigPanel(rv.cfg, rv.cfgPath, rv.logger)
	endRow := rv.ConfigPanel.Build(1)
	rv.captureRow = endRow

	// Capture preview placement
	rv.CapturePrev = NewCapturePreview(rv.captureRow)
}

// SetStateLabel updates the state label text.
func (rv *RootView) SetStateLabel(text string) {
	if rv != nil && rv.StateLabel != nil {
		rv.StateLabel.Configure(Txt(text))
	}
}

// SetConfigEditable toggles config panel editability.
func (rv *RootView) SetConfigEditable(enabled bool) {
	if rv != nil && rv.ConfigPanel != nil {
		rv.ConfigPanel.SetEditable(enabled)
	}
}

// UpdateCapture proxies to underlying capture preview view.
func (rv *RootView) UpdateCapture(img image.Image) {
	if rv != nil && rv.CapturePrev != nil {
		rv.CapturePrev.UpdateCapture(img)
	}
}

// UpdateDetection proxies to underlying capture preview view.
func (rv *RootView) UpdateDetection(img image.Image) {
	if rv != nil && rv.CapturePrev != nil {
		rv.CapturePrev.UpdateDetection(img)
	}
}

// SetSession updates both session and total capture durations.
func (rv *RootView) SetSession(seconds int, totalSeconds int) {
	if rv == nil || rv.Session == nil {
		return
	}
	rv.Session.SetSession(seconds)
	rv.Session.SetTotal(totalSeconds)
}

// --- CapturePresenter view contract methods ---
// PreviewReset clears the capture preview canvas.
func (rv *RootView) PreviewReset() {
	if rv != nil && rv.CapturePrev != nil {
		rv.CapturePrev.Reset()
	}
}

// ConfigEditable redirects to SetConfigEditable to satisfy CaptureView interface.
func (rv *RootView) ConfigEditable(b bool) { rv.SetConfigEditable(b) }
