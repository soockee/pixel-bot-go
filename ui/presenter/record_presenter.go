package presenter

import (
	"context"
	"sync/atomic"
)

// RecordJob is a prepared recording.
type RecordJob struct {
	Dir string
	Run func(context.Context) error
}

// RecordView shows the recording state.
type RecordView interface {
	SetRecording(dir string) // dir is the active recording, "" when none
	SetStatus(string)
}

// RecordPresenter starts and stops recordings from the UI. A recording runs on
// its own goroutine, and Tick reports the result on the Tk thread.
type RecordPresenter struct {
	prepare func() (RecordJob, error)
	view    RecordView
	dir     string
	cancel  context.CancelFunc // non-nil while recording
	done    chan error
	stop    atomic.Bool
}

// NewRecordPresenter returns a RecordPresenter. prepare validates the current
// settings.
func NewRecordPresenter(prepare func() (RecordJob, error), view RecordView) *RecordPresenter {
	return &RecordPresenter{prepare: prepare, view: view}
}

// Toggle starts a recording, or stops the one in progress.
func (p *RecordPresenter) Toggle() {
	if p.cancel != nil {
		p.cancel()
		return
	}
	job, err := p.prepare()
	if err != nil {
		p.view.SetStatus("Cannot record: " + err.Error())
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel, p.dir, p.done = cancel, job.Dir, make(chan error, 1)
	go func() { p.done <- job.Run(ctx) }()
	p.view.SetRecording(job.Dir)
	p.view.SetStatus("Recording to " + job.Dir)
}

// RequestStop stops the recording from any goroutine, such as the stop hotkey.
func (p *RecordPresenter) RequestStop() { p.stop.Store(true) }

// Tick applies a stop request and reports a finished recording.
func (p *RecordPresenter) Tick() {
	stop := p.stop.Swap(false)
	if p.cancel == nil {
		return
	}
	if stop {
		p.cancel()
	}
	select {
	case err := <-p.done:
		p.finish(err)
	default:
	}
}

// Stop ends the recording and waits for it, for use when the app exits.
func (p *RecordPresenter) Stop() {
	if p.cancel != nil {
		p.cancel()
		p.finish(<-p.done)
	}
}

func (p *RecordPresenter) finish(err error) {
	p.cancel()
	p.cancel = nil
	p.view.SetRecording("")
	if err != nil {
		p.view.SetStatus("Recording failed: " + err.Error())
		return
	}
	p.view.SetStatus("Saved recording to " + p.dir)
}
