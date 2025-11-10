package presenter

import (
	"errors"
	"image"
	"math"
	"sync"
	"time"

	"log/slog"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/capture"
	"github.com/soocke/pixel-bot-go/domain/fishing"
	"github.com/soocke/pixel-bot-go/ui/images"
	"github.com/soocke/pixel-bot-go/ui/model"
)

// FrameSource supplies the most recent frame captured from the game window.
type FrameSource interface {
	Running() bool
	LatestFrame() capture.FrameSnapshot
}

// DetectionFSM exposes the minimal fishing state operations used by the presenter.
type DetectionFSM interface {
	Current() fishing.FishingState
	EventTargetAcquiredAt(x, y int)
	TargetCoordinates() (int, int, bool)
	ProcessMonitoringFrame(img *image.RGBA, now time.Time)
}

// SelectionRectProvider returns the currently active capture selection.
type SelectionRectProvider interface {
	ActiveRect() *image.Rectangle
}

// DetectionView describes the UI surface updated by the presenter.
type DetectionView interface {
	UpdateCapture(img image.Image)
	UpdateDetection(img image.Image)
}

type detectionTaskKind int

const (
	detectionTaskSearch detectionTaskKind = iota + 1
	detectionTaskMonitor
)

type detectionTask struct {
	kind         detectionTaskKind
	snapshot     capture.FrameSnapshot
	selection    image.Rectangle
	hasSelection bool
	cfg          *config.Config
	target       image.Image
	targetPoint  image.Point
}

type detectionResult struct {
	kind     detectionTaskKind
	sequence uint64
	err      error
	found    bool
	location image.Point
	roi      *image.RGBA
	roiRect  image.Rectangle
	duration time.Duration
}

// DetectionPresenter coordinates capture preview and detection scheduling.
type DetectionPresenter struct {
	Enabled   func() bool
	Source    FrameSource
	FSM       DetectionFSM
	Selection SelectionRectProvider
	View      DetectionView
	Config    *config.Config
	TargetImg image.Image
	Model     *model.DetectionModel
	logger    *slog.Logger

	workerOnce sync.Once
	workCh     chan detectionTask
	resultCh   chan detectionResult

	lastSearchSeq  uint64
	lastMonitorSeq uint64
	lastSearchTime time.Time
	searchDelay    time.Duration
}

// NewDetectionPresenter constructs a detection presenter.
func NewDetectionPresenter(enabled func() bool, source FrameSource, fsm DetectionFSM, selection SelectionRectProvider, view DetectionView, cfg *config.Config, target image.Image, model *model.DetectionModel, logger *slog.Logger) *DetectionPresenter {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	return &DetectionPresenter{
		Enabled:        enabled,
		Source:         source,
		FSM:            fsm,
		Selection:      selection,
		View:           view,
		Config:         cfg,
		TargetImg:      target,
		Model:          model,
		logger:         logger,
		workCh:         make(chan detectionTask, 1),
		resultCh:       make(chan detectionResult, 1),
		searchDelay:    65 * time.Millisecond,
		lastSearchSeq:  0,
		lastMonitorSeq: 0,
	}
}

// ProcessFrame pulls the latest frame, schedules detection work, and handles worker results.
func (p *DetectionPresenter) ProcessFrame() {
	if p == nil || p.Enabled == nil || p.Source == nil || p.FSM == nil || p.View == nil {
		return
	}

	p.ensureWorker()

	for {
		select {
		case res := <-p.resultCh:
			p.handleResult(res)
		default:
			goto drained
		}
	}

drained:
	if !p.Enabled() || !p.Source.Running() {
		return
	}

	snapshot := p.Source.LatestFrame()
	frame := snapshot.Image
	if frame == nil {
		return
	}

	p.View.UpdateCapture(frame)

	var selection image.Rectangle
	hasSelection := false
	if p.Selection != nil {
		if rect := p.Selection.ActiveRect(); rect != nil {
			selection = *rect
			hasSelection = true
		}
	}

	switch p.FSM.Current() {
	case fishing.StateSearching:
		p.maybeDispatchSearch(snapshot, selection, hasSelection)
	case fishing.StateMonitoring:
		p.maybeDispatchMonitor(snapshot, selection, hasSelection)
	}
}

func (p *DetectionPresenter) ensureWorker() {
	p.workerOnce.Do(func() {
		go p.runWorker()
	})
}

func (p *DetectionPresenter) runWorker() {
	for task := range p.workCh {
		res := p.executeTask(task)
		if res.kind == 0 {
			continue
		}
		select {
		case p.resultCh <- res:
		default:
			select {
			case <-p.resultCh:
			default:
			}
			select {
			case p.resultCh <- res:
			default:
			}
		}
	}
}

func (p *DetectionPresenter) maybeDispatchSearch(snapshot capture.FrameSnapshot, selection image.Rectangle, hasSelection bool) {
	if p.TargetImg == nil {
		return
	}
	if snapshot.Sequence == 0 || snapshot.Sequence == p.lastSearchSeq {
		return
	}
	if !p.lastSearchTime.IsZero() && time.Since(p.lastSearchTime) < p.searchDelay {
		return
	}
	p.lastSearchSeq = snapshot.Sequence
	p.lastSearchTime = time.Now()
	task := detectionTask{
		kind:         detectionTaskSearch,
		snapshot:     snapshot,
		selection:    selection,
		hasSelection: hasSelection,
		cfg:          p.copyConfig(),
		target:       p.TargetImg,
	}
	p.dispatchTask(task)
}

func (p *DetectionPresenter) maybeDispatchMonitor(snapshot capture.FrameSnapshot, selection image.Rectangle, hasSelection bool) {
	if snapshot.Sequence == 0 || snapshot.Sequence == p.lastMonitorSeq {
		return
	}
	px, py, ok := p.FSM.TargetCoordinates()
	if !ok {
		return
	}
	p.lastMonitorSeq = snapshot.Sequence
	task := detectionTask{
		kind:         detectionTaskMonitor,
		snapshot:     snapshot,
		selection:    selection,
		hasSelection: hasSelection,
		cfg:          p.copyConfig(),
		targetPoint:  image.Pt(px, py),
	}
	p.dispatchTask(task)
}

func (p *DetectionPresenter) dispatchTask(task detectionTask) {
	select {
	case p.workCh <- task:
	default:
		select {
		case <-p.workCh:
		default:
		}
		select {
		case p.workCh <- task:
		default:
		}
	}
}

func (p *DetectionPresenter) executeTask(task detectionTask) detectionResult {
	res := detectionResult{kind: task.kind, sequence: task.snapshot.Sequence}
	frame := task.snapshot.Image
	if frame == nil {
		res.err = errors.New("nil frame")
		return res
	}
	cfg := task.cfg
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	switch task.kind {
	case detectionTaskSearch:
		return p.doSearch(task, frame, cfg)
	case detectionTaskMonitor:
		return p.doMonitor(task, frame, cfg)
	default:
		res.err = errors.New("unknown detection task kind")
		return res
	}
}

func (p *DetectionPresenter) doSearch(task detectionTask, frame *image.RGBA, cfg *config.Config) detectionResult {
	res := detectionResult{kind: detectionTaskSearch, sequence: task.snapshot.Sequence}
	analysis := frame
	scaleX, scaleY := 1.0, 1.0
	if cfg.AnalysisScale > 0 && cfg.AnalysisScale < 1.0 {
		w := int(math.Max(1, math.Round(float64(frame.Bounds().Dx())*cfg.AnalysisScale)))
		h := int(math.Max(1, math.Round(float64(frame.Bounds().Dy())*cfg.AnalysisScale)))
		scaled := images.ScaleToFit(frame, w, h)
		if scaled != nil && scaled.Bounds().Dx() > 0 && scaled.Bounds().Dy() > 0 {
			analysis = scaled
			scaleX = float64(frame.Bounds().Dx()) / float64(analysis.Bounds().Dx())
			scaleY = float64(frame.Bounds().Dy()) / float64(analysis.Bounds().Dy())
		}
	}
	start := time.Now()
	match, err := capture.DetectTemplateDetailed(analysis, task.target, cfg)
	res.duration = time.Since(start)
	if err != nil {
		res.err = err
		return res
	}
	if !match.Found {
		return res
	}
	x := match.X
	y := match.Y
	if analysis != frame {
		x = int(math.Round(float64(x) * scaleX))
		y = int(math.Round(float64(y) * scaleY))
	}
	if task.hasSelection {
		x += task.selection.Min.X
		y += task.selection.Min.Y
	}
	res.found = true
	res.location = image.Pt(x, y)
	return res
}

func (p *DetectionPresenter) doMonitor(task detectionTask, frame *image.RGBA, cfg *config.Config) detectionResult {
	res := detectionResult{kind: detectionTaskMonitor, sequence: task.snapshot.Sequence}
	pt := task.targetPoint
	localX := pt.X
	localY := pt.Y
	if task.hasSelection {
		localX -= task.selection.Min.X
		localY -= task.selection.Min.Y
	}
	roi, rect, err := images.ExtractROI(frame, localX, localY, cfg.ROISizePx)
	if err != nil {
		res.err = err
		return res
	}
	globalRect := rect
	if task.hasSelection {
		globalRect = rect.Add(task.selection.Min)
	}
	res.found = true
	res.location = pt
	res.roi = roi
	res.roiRect = globalRect
	return res
}

func (p *DetectionPresenter) handleResult(res detectionResult) {
	if res.err != nil {
		if p.logger != nil {
			p.logger.Error("detection", "error", res.err)
		}
		return
	}
	switch res.kind {
	case detectionTaskSearch:
		if res.found {
			p.FSM.EventTargetAcquiredAt(res.location.X, res.location.Y)
		}
	case detectionTaskMonitor:
		if res.roi != nil {
			if p.Model != nil {
				p.Model.SetROI(res.roiRect)
			}
			p.View.UpdateDetection(res.roi)
			p.FSM.ProcessMonitoringFrame(res.roi, time.Now())
		}
	}
}

func (p *DetectionPresenter) copyConfig() *config.Config {
	if p.Config == nil {
		return config.DefaultConfig()
	}
	clone := *p.Config
	return &clone
}
