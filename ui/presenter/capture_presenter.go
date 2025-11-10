package presenter

import (
	"github.com/soocke/pixel-bot-go/domain/capture"
)

// CaptureModel provides access to the capture enabled state.
type CaptureModel interface {
	Enabled() bool
	SetEnabled(bool)
}

// LifecycleContract is the minimal lifecycle API used by the presenter.
type LifecycleContract interface {
	Start()
	Stop()
}

// CaptureFSM provides the events the presenter needs to interact with the FSM.
type CaptureFSM interface {
	EventAwaitFocus()
	EventHalt()
}

// CaptureView updates UI elements affected by capture toggling.
// The presenter does not manage state-label updates; that responsibility
// belongs to the FSMPresenter.
type CaptureView interface {
	PreviewReset()
	ConfigEditable(bool)
}

// CapturePresenter coordinates capture enable/disable actions between the
// model, capture service, FSM and view.
type CapturePresenter struct {
	model   CaptureModel
	service LifecycleContract
	fsm     CaptureFSM
	view    CaptureView
}

func NewCapturePresenter(model CaptureModel, service capture.CaptureService, fsm CaptureFSM, view CaptureView) *CapturePresenter {
	return &CapturePresenter{model: model, service: service, fsm: fsm, view: view}
}

// Toggle flips enabled state, coordinating service, FSM and view.
// Enable starts the capture service and triggers the FSM to await focus. Idempotent.
func (c *CapturePresenter) Enable() {
	if c == nil || c.model == nil || c.service == nil || c.view == nil || c.fsm == nil {
		return
	}
	if c.model.Enabled() { // already enabled
		return
	}
	c.service.Start()
	c.model.SetEnabled(true)
	c.fsm.EventAwaitFocus()
	c.view.ConfigEditable(false)
}

// Disable stops the capture service and halts the FSM, resetting preview. Idempotent.
func (c *CapturePresenter) Disable() {
	if c == nil || c.model == nil || c.service == nil || c.view == nil || c.fsm == nil {
		return
	}
	if !c.model.Enabled() { // already disabled
		return
	}
	c.service.Stop()
	c.model.SetEnabled(false)
	c.view.PreviewReset()
	c.fsm.EventHalt()
	c.view.ConfigEditable(true)
}

// Toggle flips enabled state delegating to Enable/Disable.
func (c *CapturePresenter) Toggle() {
	if c == nil || c.model == nil || c.service == nil || c.view == nil || c.fsm == nil {
		return
	}
	if c.model.Enabled() {
		c.Disable()
		return
	}
	c.Enable()
}
