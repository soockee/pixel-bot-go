package presenter

import (
	"github.com/soocke/pixel-bot-go/domain/capture"
)

// CaptureModel provides enabled state access.
type CaptureModel interface {
	Enabled() bool
	SetEnabled(bool)
}

// LifecycleContract narrows what presenter needs from the capture layer.
type LifecycleContract interface {
	Start()
	Stop()
}

// CaptureFSM exposes focus & halt events directly (align with FishingFSM names).
type CaptureFSM interface {
	EventAwaitFocus()
	EventHalt()
}

// CaptureView updates UI elements affected by capture toggling.
// State label updates are now owned solely by FSMPresenter; this presenter
// no longer mutates it directly to preserve single responsibility.
type CaptureView interface {
	PreviewReset()
	ConfigEditable(bool)
}

// CapturePresenter owns presentation logic for toggling capture state.
type CapturePresenter struct {
	model   CaptureModel
	service LifecycleContract // narrowed from full capture.CaptureService
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
