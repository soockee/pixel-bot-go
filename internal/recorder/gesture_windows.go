package recorder

import (
	"github.com/soocke/pixel-bot-go/internal/playtrace"
	"math"
)

const (
	dragPixels = 6
	dragCounts = 8
)

var mouseButtons = [...]string{"left", "right", "middle", "x1", "x2"}

type mouseGesture = playtrace.Gesture

type buttonHold struct {
	active  bool
	started int64
	gesture mouseGesture
}

type gestureTracker struct {
	inClient bool
	holds    [len(mouseButtons)]buttonHold
}

func (s *gestureTracker) packet(raw rawMouse, point clientPosition, now int64, segment int, emit func(inputEvent)) {
	s.inClient = true
	for i, button := range mouseButtons {
		if raw.ButtonFlags&(1<<(i*2)) != 0 {
			if s.holds[i].active {
				s.finish(i, now, segment, true, emit)
			}
			s.holds[i] = buttonHold{active: true, started: now, gesture: mouseGesture{Start: point, End: point}}
			emit(inputEvent{Type: "button_down", ElapsedUS: now, Segment: segment, Button: button, Position: &point})
		}
	}

	dx, dy := raw.DX, raw.DY
	if raw.Flags&1 != 0 { // Absolute packets supply positions instead of device-relative counts.
		dx, dy = 0, 0
	}
	travel := math.Hypot(float64(dx), float64(dy))
	for i := range s.holds {
		if hold := &s.holds[i]; hold.active {
			g := &hold.gesture
			g.End = point
			if point.Width == g.Start.Width && point.Height == g.Start.Height {
				g.MaxClientPixels = max(g.MaxClientPixels, math.Hypot(float64(point.X-g.Start.X), float64(point.Y-g.Start.Y)))
			}
			g.RawDX += int64(dx)
			g.RawDY += int64(dy)
			g.RawTravel += travel
			g.MaxRawCounts = max(g.MaxRawCounts, math.Hypot(float64(g.RawDX), float64(g.RawDY)))
		}
	}
	if raw.DX != 0 || raw.DY != 0 {
		emit(inputEvent{Type: "move", ElapsedUS: now, Segment: segment, Position: &point, DX: dx, DY: dy})
	}
	for i, button := range mouseButtons {
		if raw.ButtonFlags&(2<<(i*2)) != 0 {
			emit(inputEvent{Type: "button_up", ElapsedUS: now, Segment: segment, Button: button, Position: &point})
			if s.holds[i].active {
				s.finish(i, now, segment, false, emit)
			}
		}
	}
	if raw.ButtonFlags&0x0400 != 0 {
		emit(inputEvent{Type: "wheel", ElapsedUS: now, Segment: segment, Position: &point, Wheel: int16(raw.ButtonData)})
	}
	if raw.ButtonFlags&0x0800 != 0 {
		emit(inputEvent{Type: "wheel_horizontal", ElapsedUS: now, Segment: segment, Position: &point, Wheel: int16(raw.ButtonData)})
	}
}

func (s *gestureTracker) finish(i int, now int64, segment int, interrupted bool, emit func(inputEvent)) {
	hold := s.holds[i]
	s.holds[i] = buttonHold{}
	g := hold.gesture
	g.DurationUS = now - hold.started
	kind := "click"
	switch {
	case interrupted:
		kind = "gesture_interrupted"
	case g.MaxClientPixels >= dragPixels || g.MaxRawCounts >= dragCounts:
		kind = "drag"
	}
	emit(inputEvent{Type: kind, ElapsedUS: now, Segment: segment, Button: mouseButtons[i], Position: &g.End, Gesture: &g})
}

func (s *gestureTracker) leave(now int64, segment int, emit func(inputEvent)) {
	if !s.inClient {
		return
	}
	s.inClient = false
	for i, hold := range s.holds {
		if hold.active {
			s.finish(i, now, segment, true, emit)
		}
	}
	emit(inputEvent{Type: "mouse_gap", ElapsedUS: now, Segment: segment})
}
