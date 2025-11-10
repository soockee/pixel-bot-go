package presenter

import (
	"image"
	"log/slog"
	"time"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/capture"
	"github.com/soocke/pixel-bot-go/domain/fishing" // Placeholder for alignment
	"github.com/soocke/pixel-bot-go/ui/images"
	"github.com/soocke/pixel-bot-go/ui/model"
)

// FrameSource supplies capture frames.
type FrameSource interface {
	Running() bool
	Frames() <-chan *image.RGBA
}

// DetectionFSM abstracts required FSM interactions.
type DetectionFSM interface {
	Current() fishing.FishingState
	EventTargetAcquiredAt(int, int)
	TargetCoordinates() (int, int, bool)
	ProcessMonitoringFrame(*image.RGBA, time.Time)
}

// SelectionRectProvider supplies optional selection rectangle.
// SelectionRectProvider supplies optional selection rectangle.
// Method name aligned with view.SelectionOverlay.ActiveRect for direct injection (no adapter).
type SelectionRectProvider interface{ ActiveRect() *image.Rectangle }

// DetectionView updates capture + ROI preview.
// DetectionView updates capture + ROI preview. UpdateDetection now accepts image.Image
// to avoid unnecessary adapter wrapping (*image.RGBA still satisfies image.Image).
type DetectionView interface {
	UpdateCapture(image.Image)
	UpdateDetection(image.Image)
}

// DetectionPresenter drives detection & ROI extraction, updating model & view.
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
}

func NewDetectionPresenter(enabled func() bool, src FrameSource, fsm DetectionFSM, sel SelectionRectProvider, view DetectionView, cfg *config.Config, target image.Image, model *model.DetectionModel) *DetectionPresenter {
	return &DetectionPresenter{Enabled: enabled, Source: src, FSM: fsm, Selection: sel, View: view, Config: cfg, TargetImg: target, Model: model}
}

// ProcessFrame reads a frame (non-blocking) and processes detection logic based on FSM state.
func (p *DetectionPresenter) ProcessFrame() {
	if p == nil || p.Source == nil || p.FSM == nil || p.View == nil || p.Enabled == nil || p.Model == nil {
		return
	}
	if !p.Enabled() || !p.Source.Running() {
		return
	}
	fsmState := p.FSM.Current()
	if fsmState == fishing.StateWaitingFocus {
		return
	}
	frame := <-p.Source.Frames()
	if frame == nil {
		return
	}
	// Display scaled preview only; analysis may use original or downscaled copy.
	p.View.UpdateCapture(frame) // preview scaling handled by view
	// Optional analysis frame scaling for template matching (searching state only).
	analysisFrame := frame
	var scale float64 = 1.0
	if p.Config != nil && p.Config.AnalysisScale > 0 && p.Config.AnalysisScale < 1.0 {
		// Use ScaleToFit to approximate uniform scale reduction.
		b := frame.Bounds()
		w := int(float64(b.Dx()) * p.Config.AnalysisScale)
		h := int(float64(b.Dy()) * p.Config.AnalysisScale)
		if w < 1 {
			w = 1
		}
		if h < 1 {
			h = 1
		}
		scaled := images.ScaleToFit(frame, w, h)
		if rgba, ok := scaled.(*image.RGBA); ok {
			analysisFrame = rgba
			scale = p.Config.AnalysisScale
		}
	}
	if fsmState == fishing.StateSearching && p.TargetImg != nil {
		x, y, ok, err := capture.DetectTemplate(analysisFrame, p.TargetImg, p.Config)
		if err != nil {
			p.logger.Error("detectTemplate", "error", err)
		}
		if ok {
			// Translate coordinates back to original frame space if scaled.
			if scale != 1.0 {
				x = int(float64(x)/scale + 0.5)
				y = int(float64(y)/scale + 0.5)
			}
			if sel := p.Selection.ActiveRect(); sel != nil {
				x += sel.Min.X
				y += sel.Min.Y
			}
			p.FSM.EventTargetAcquiredAt(x, y)
		}
	} else if fsmState == fishing.StateMonitoring {
		// Monitoring retains original frame for ROI extraction fidelity.
		x, y, ok := p.FSM.TargetCoordinates()
		if ok {
			if sel := p.Selection.ActiveRect(); sel != nil { // convert global to frame-local
				x -= sel.Min.X
				y -= sel.Min.Y
			}
			roiImg, roiRect, err := images.ExtractROI(frame, x, y, p.Config.ROISizePx)
			if err != nil || roiImg == nil {
				return
			}
			if sel := p.Selection.ActiveRect(); sel != nil { // store global rect
				p.Model.SetROI(image.Rect(sel.Min.X+roiRect.Min.X, sel.Min.Y+roiRect.Min.Y, sel.Min.X+roiRect.Max.X, sel.Min.Y+roiRect.Max.Y))
			} else {
				p.Model.SetROI(roiRect)
			}
			p.View.UpdateDetection(roiImg)
			p.FSM.ProcessMonitoringFrame(roiImg, time.Now())
		}
	}
	// Recycle original frame after all processing to enable buffer reuse.
	// Safe because we no longer access 'frame' after this point.
	capture.RecycleFrame(frame)
}
