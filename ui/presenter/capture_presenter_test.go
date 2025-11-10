package presenter

import (
	"image"
	"testing"

	cap "github.com/soocke/pixel-bot-go/domain/capture"
)

type mockModel struct{ enabled bool }

func (m *mockModel) Enabled() bool     { return m.enabled }
func (m *mockModel) SetEnabled(b bool) { m.enabled = b }

// mockService implements a minimal subset + Frames to satisfy capture.CaptureService
type mockService struct{ started, stopped int }

func (s *mockService) Start()                                       { s.started++ }
func (s *mockService) Stop()                                        { s.stopped++ }
func (s *mockService) LatestFrame() cap.FrameSnapshot               { return cap.FrameSnapshot{} }
func (s *mockService) Running() bool                                { return s.started > s.stopped }
func (s *mockService) SetSelectionProvider(func() *image.Rectangle) {}
func (s *mockService) Stats() cap.CaptureStats                      { return cap.CaptureStats{} }

var _ cap.CaptureService = (*mockService)(nil)

type mockFSM struct{ awaited, halted int }

func (f *mockFSM) EventAwaitFocus() { f.awaited++ }
func (f *mockFSM) EventHalt()       { f.halted++ }

type mockView struct {
	reset, editableCalls int
	lastEditable         bool
}

func (v *mockView) PreviewReset()         { v.reset++ }
func (v *mockView) ConfigEditable(b bool) { v.editableCalls++; v.lastEditable = b }

func TestCapturePresenter_EnableDisable_Idempotent(t *testing.T) {
	m := &mockModel{}
	svc := &mockService{}
	fsm := &mockFSM{}
	view := &mockView{}
	p := NewCapturePresenter(m, svc, fsm, view)

	// Enable
	p.Enable()
	if !m.Enabled() || svc.started != 1 || fsm.awaited != 1 || view.lastEditable || view.editableCalls != 1 {
		t.Fatalf("enable failed: enabled=%v started=%d awaited=%d editableCalls=%d lastEditable=%v", m.Enabled(), svc.started, fsm.awaited, view.editableCalls, view.lastEditable)
	}
	// Enable again idempotent
	p.Enable()
	if svc.started != 1 || fsm.awaited != 1 {
		t.Fatalf("enable not idempotent: started=%d awaited=%d", svc.started, fsm.awaited)
	}

	// Disable
	p.Disable()
	if m.Enabled() || svc.stopped != 1 || fsm.halted != 1 || view.reset != 1 || !view.lastEditable || view.editableCalls != 2 {
		t.Fatalf("disable failed: enabled=%v stopped=%d halted=%d reset=%d editableCalls=%d lastEditable=%v", m.Enabled(), svc.stopped, fsm.halted, view.reset, view.editableCalls, view.lastEditable)
	}
	// Disable again idempotent
	p.Disable()
	if svc.stopped != 1 || fsm.halted != 1 || view.reset != 1 {
		t.Fatalf("disable not idempotent: stopped=%d halted=%d reset=%d", svc.stopped, fsm.halted, view.reset)
	}
}

func TestCapturePresenter_Toggle(t *testing.T) {
	m := &mockModel{}
	svc := &mockService{}
	fsm := &mockFSM{}
	view := &mockView{}
	p := NewCapturePresenter(m, svc, fsm, view)
	p.Toggle() // enable path
	if !m.Enabled() || svc.started != 1 || fsm.awaited != 1 {
		t.Fatalf("toggle enable failed")
	}
	p.Toggle() // disable path
	if m.Enabled() || svc.stopped != 1 || fsm.halted != 1 || view.reset != 1 {
		t.Fatalf("toggle disable failed")
	}
}
