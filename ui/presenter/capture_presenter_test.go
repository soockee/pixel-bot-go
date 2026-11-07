package presenter

import (
	"errors"
	"testing"

	"github.com/soocke/pixel-bot-go/domain/fishing"
)

type mockModel struct{ enabled bool }

func (m *mockModel) Enabled() bool     { return m.enabled }
func (m *mockModel) SetEnabled(b bool) { m.enabled = b }

type mockPipeline struct {
	started, stopped int
	err              error
}

func (s *mockPipeline) Start() error {
	if s.err == nil {
		s.started++
	}
	return s.err
}
func (s *mockPipeline) Stop() { s.stopped++ }

type mockFSM struct{ awaited, halted int }

func (f *mockFSM) EventAwaitFocus() { f.awaited++ }
func (f *mockFSM) EventHalt()       { f.halted++ }

type mockView struct {
	reset   int
	running bool
	status  string
}

func (v *mockView) PreviewReset()      { v.reset++ }
func (v *mockView) SetRunning(b bool)  { v.running = b }
func (v *mockView) SetStatus(s string) { v.status = s }

func TestCapturePresenterEnableDisableAreIdempotent(t *testing.T) {
	m := &mockModel{}
	pipeline := &mockPipeline{}
	fsm := &mockFSM{}
	view := &mockView{}
	p := NewCapturePresenter(m, pipeline, fsm, view)

	p.Enable()
	p.Enable()
	if !m.Enabled() || pipeline.started != 1 || fsm.awaited != 1 || !view.running {
		t.Fatalf("enable: enabled=%v started=%d awaited=%d running=%v", m.Enabled(), pipeline.started, fsm.awaited, view.running)
	}

	p.Disable()
	p.Disable()
	if m.Enabled() || pipeline.stopped != 1 || fsm.halted != 1 || view.reset != 1 || view.running {
		t.Fatalf("disable: enabled=%v stopped=%d halted=%d reset=%d running=%v", m.Enabled(), pipeline.stopped, fsm.halted, view.reset, view.running)
	}
}

func TestCapturePresenterStopsCaptureAfterFSMHalt(t *testing.T) {
	m := &mockModel{}
	pipeline := &mockPipeline{}
	p := NewCapturePresenter(m, pipeline, &mockFSM{}, &mockView{})
	p.Enable()

	p.Tick()
	if !m.Enabled() {
		t.Fatal("capture stopped without a halt")
	}

	p.OnState(fishing.StateCooldown, fishing.StateHalt)
	p.Tick()
	if m.Enabled() || pipeline.stopped != 1 {
		t.Fatalf("capture still running: enabled=%v stopped=%d", m.Enabled(), pipeline.stopped)
	}
}

func TestCapturePresenterShowsWhyItCannotStart(t *testing.T) {
	m := &mockModel{}
	fsm := &mockFSM{}
	view := &mockView{}
	p := NewCapturePresenter(m, &mockPipeline{err: errors.New("unknown locator")}, fsm, view)

	p.Enable()

	if m.Enabled() || fsm.awaited != 0 || view.running || view.status != "Cannot start: unknown locator" {
		t.Errorf("enabled=%v awaited=%d running=%v status=%q", m.Enabled(), fsm.awaited, view.running, view.status)
	}
}

func TestCapturePresenterToggle(t *testing.T) {
	m := &mockModel{}
	pipeline := &mockPipeline{}
	p := NewCapturePresenter(m, pipeline, &mockFSM{}, &mockView{})

	p.Toggle()
	if !m.Enabled() || pipeline.started != 1 {
		t.Fatal("toggle did not enable")
	}
	p.Toggle()
	if m.Enabled() || pipeline.stopped != 1 {
		t.Fatal("toggle did not disable")
	}
}
