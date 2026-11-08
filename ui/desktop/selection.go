package desktop

import (
	"image"
	"log/slog"
	"sync/atomic"

	"github.com/soocke/pixel-bot-go/config"
)

// Selection holds the capture area and saves it to the config. ActiveRect is
// safe from any goroutine; the other methods run on the UI loop.
type Selection struct {
	cfg    *config.Config
	path   string
	logger *slog.Logger
	rect   atomic.Pointer[image.Rectangle]
}

// NewSelection returns the selection saved in cfg.
func NewSelection(cfg *config.Config, path string, logger *slog.Logger) *Selection {
	s := &Selection{cfg: cfg, path: path, logger: logger}
	if cfg.SelectionW > 0 && cfg.SelectionH > 0 {
		r := image.Rect(cfg.SelectionX, cfg.SelectionY, cfg.SelectionX+cfg.SelectionW, cfg.SelectionY+cfg.SelectionH)
		s.rect.Store(&r)
	}
	return s
}

// ActiveRect returns the selected rectangle, or nil.
func (s *Selection) ActiveRect() *image.Rectangle { return s.rect.Load() }

// Set selects r and saves it.
func (s *Selection) Set(r image.Rectangle) {
	r = r.Canon()
	if r.Empty() {
		return
	}
	s.rect.Store(&r)
	s.cfg.SelectionX, s.cfg.SelectionY, s.cfg.SelectionW, s.cfg.SelectionH = r.Min.X, r.Min.Y, r.Dx(), r.Dy()
	s.save()
}

// Clear removes the selection and saves that.
func (s *Selection) Clear() {
	s.rect.Store(nil)
	s.cfg.SelectionW, s.cfg.SelectionH = 0, 0
	s.save()
}

// Start returns where the overlay opens: the selection, or a centred area of
// screen when there is none.
func (s *Selection) Start(screen image.Rectangle) image.Rectangle {
	if r := s.ActiveRect(); r != nil {
		return *r
	}
	w, h := max(screen.Dx()*2/3, 1), max(screen.Dy()*5/9, 1)
	x, y := screen.Min.X+(screen.Dx()-w)/2, screen.Min.Y+(screen.Dy()-h)/2
	return image.Rect(x, y, x+w, y+h)
}

func (s *Selection) save() {
	if err := s.cfg.Save(s.path); err != nil {
		s.logger.Error("save selection failed", "error", err)
	}
}
