package app

import (
	"image"
	"log/slog"

	"github.com/soocke/pixel-bot-go/assets"
	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/domain/capture"
	"github.com/soocke/pixel-bot-go/domain/fishing"
	"github.com/soocke/pixel-bot-go/ui/model"
	"github.com/soocke/pixel-bot-go/ui/presenter"
	"github.com/soocke/pixel-bot-go/ui/view"
)

// Container assembles models, services, presenters and the root view.
type AppContainer struct {
	Config     *config.Config
	Logger     *slog.Logger
	Capture    *model.CaptureModel
	Session    *model.SessionModel
	Detection  *model.DetectionModel
	CaptureSvc capture.CaptureService
	FSM        fishing.FishingFSMContract
	RootView   *view.RootView
	UI         view.UI

	// Presenters
	SessionPresenter   *presenter.SessionPresenter
	FSMPresenter       *presenter.FSMPresenter
	DetectionPresenter *presenter.DetectionPresenter
	CapturePresenter   *presenter.CapturePresenter
	Loop               *presenter.Loop
	TargetImg          image.Image
}

// BuildContainer constructs all components. Side-effects limited to asset loading.
func BuildContainer(cfg *config.Config, logger *slog.Logger, width, height int, cfgPath string) *AppContainer {
	c := &AppContainer{Config: cfg, Logger: logger}
	c.Capture = &model.CaptureModel{}
	c.Session = model.NewSessionModel()
	c.Detection = model.NewDetectionModel()
	c.CaptureSvc = capture.NewCaptureService(logger, func() *image.Rectangle { return nil })
	if img, err := assets.FishingTargetImage(); err == nil {
		c.TargetImg = img
	}
	c.FSM = fishing.NewFSM(logger, cfg, fishing.ActionCallbacks{
		PressKey:   action.PressKey,
		MoveCursor: action.MoveCursor,
		ClickRight: action.ClickRight,
		ParseVK:    action.ParseVK,
	}, func(cfg *config.Config, l *slog.Logger) fishing.BiteDetectorContract {
		return fishing.NewBiteDetector(cfg, l)
	})
	// View
	c.RootView = view.NewRootView(cfg, cfgPath, logger)
	// UI built externally after window list retrieval.
	c.UI = c.RootView
	// Presenters wired after UI & FSM ready (selection / adapters resolved by app wrapper).
	return c
}
