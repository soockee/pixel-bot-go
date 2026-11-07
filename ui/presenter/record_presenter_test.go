package presenter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
)

type mockRecordView struct {
	recording bool
	status    string
}

func (v *mockRecordView) SetRecording(b bool) { v.recording = b }
func (v *mockRecordView) SetStatus(s string)  { v.status = s }

func blockingJob(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func newRecordPresenter(run func(context.Context) error) (*RecordPresenter, *mockRecordView) {
	view := &mockRecordView{}
	prepare := func() (RecordJob, error) { return RecordJob{Dir: "datasets/a", Run: run}, nil }
	return NewRecordPresenter(prepare, view, "127.0.0.1:6070"), view
}

func TestRecordPresenterStartsAndStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, view := newRecordPresenter(blockingJob)

		p.Toggle()
		if !view.recording || !strings.Contains(view.status, "127.0.0.1:6070") {
			t.Fatalf("recording=%v status=%q", view.recording, view.status)
		}

		p.Toggle()
		synctest.Wait()
		p.Tick()
		if view.recording || !strings.Contains(view.status, "Saved recording to datasets/a") {
			t.Fatalf("recording=%v status=%q", view.recording, view.status)
		}
	})
}

func TestRecordPresenterShowsWhyItCannotStart(t *testing.T) {
	view := &mockRecordView{}
	p := NewRecordPresenter(func() (RecordJob, error) { return RecordJob{}, errors.New("no window") }, view, "x")

	p.Toggle()

	if view.recording || view.status != "Cannot record: no window" {
		t.Errorf("recording=%v status=%q", view.recording, view.status)
	}
}

func TestRecordPresenterReportsAFailedRecording(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, view := newRecordPresenter(func(context.Context) error { return errors.New("port in use") })

		p.Toggle()
		synctest.Wait()
		p.Tick()

		if view.recording || view.status != "Recording failed: port in use" {
			t.Errorf("recording=%v status=%q", view.recording, view.status)
		}
	})
}

func TestRecordPresenterStopsFromTheHotkey(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, view := newRecordPresenter(blockingJob)
		p.RequestStop() // nothing is recording, so this must not stop the next one
		p.Tick()
		p.Toggle()

		p.Tick()
		if !view.recording {
			t.Fatal("an old stop request ended the recording")
		}

		p.RequestStop()
		p.Tick()
		synctest.Wait()
		p.Tick()
		if view.recording {
			t.Error("still recording")
		}
	})
}

func TestRecordPresenterStopWaitsForTheRecording(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, view := newRecordPresenter(blockingJob)
		p.Toggle()

		p.Stop()
		p.Stop()

		if view.recording {
			t.Error("still recording")
		}
	})
}
