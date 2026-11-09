package presenter

import (
	"sync/atomic"

	"github.com/soocke/pixel-bot-go/domain/fishing"
)

// CaptureModel provides access to the capture enabled state.
type CaptureModel interface {
	Enabled() bool
	SetEnabled(bool)
}

// Lifecycle is the pipeline the presenter starts and stops.
type Lifecycle interface {
	Start() error
	Stop()
}

// CaptureFSM provides the events the presenter needs to interact with the FSM.
type CaptureFSM interface {
	EventAwaitFocus()
	EventHalt()
	HaltReason() string // why the run halted on its own, "" when the person stopped it
}

// CaptureView updates UI elements affected by capture toggling.
type CaptureView interface {
	PreviewReset()
	SetRunning(bool)
	SetStatus(string)
}

// CapturePresenter coordinates capture enable/disable actions between the
// model, pipeline, FSM and view.
type CapturePresenter struct {
	model    CaptureModel
	pipeline Lifecycle
	fsm      CaptureFSM
	view     CaptureView
	halted   atomic.Bool // the FSM halted; Tick stops capture on the Tk thread
}

// NewCapturePresenter returns a CapturePresenter.
func NewCapturePresenter(model CaptureModel, pipeline Lifecycle, fsm CaptureFSM, view CaptureView) *CapturePresenter {
	return &CapturePresenter{model: model, pipeline: pipeline, fsm: fsm, view: view}
}

// Enable starts the pipeline and makes the FSM wait for focus. Idempotent.
func (c *CapturePresenter) Enable() {
	if c.model.Enabled() {
		return
	}
	if err := c.pipeline.Start(); err != nil {
		c.view.SetStatus("Cannot start: " + err.Error())
		return
	}
	c.model.SetEnabled(true)
	c.fsm.EventAwaitFocus()
	c.view.SetRunning(true)
}

// Disable stops the pipeline and halts the FSM, resetting the preview. Idempotent.
func (c *CapturePresenter) Disable() {
	if !c.model.Enabled() {
		return
	}
	c.fsm.EventHalt()
	c.pipeline.Stop()
	c.model.SetEnabled(false)
	c.view.PreviewReset()
	c.view.SetRunning(false)
}

// OnState records a halt from any source, such as the stop hotkey or a limit.
// Register it as an FSM listener.
func (c *CapturePresenter) OnState(_, next fishing.FishingState) {
	if next == fishing.StateHalt {
		c.halted.Store(true)
	}
}

// Tick stops capture after the FSM halted on its own, and says why when the
// FSM knows: a run that just stops leaves the person guessing.
func (c *CapturePresenter) Tick() {
	if c.halted.Swap(false) {
		reason := c.fsm.HaltReason()
		c.Disable()
		if reason != "" {
			c.view.SetStatus("Stopped: " + reason)
		}
	}
}

// Toggle flips enabled state delegating to Enable/Disable.
func (c *CapturePresenter) Toggle() {
	if c.model.Enabled() {
		c.Disable()
		return
	}
	c.Enable()
}
