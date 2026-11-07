package main

import (
	"math"

	"github.com/soocke/pixel-bot-go/internal/playtrace"
)

const movementGapUS = 250_000

var activityKinds = [...]string{"click", "drag", "key", "shortcut", "wheel", "free_move_burst"}

type activityProfile struct {
	Counts             map[string]int `json:"counts"`
	Actions            int            `json:"actions"`
	MixScore           *float64       `json:"mix_score,omitempty"`
	KeyboardCaptured   bool           `json:"keyboard_captured"`
	ClosedCaptureUS    int64          `json:"closed_capture_us"`
	ActionsPerMinute   *float64       `json:"actions_per_capture_minute,omitempty"`
	RightDrags         int            `json:"right_drags"`
	RightDragHoldUS    int64          `json:"right_drag_hold_us"`
	DragRawTravel      float64        `json:"drag_raw_travel"`
	RightDragRawTravel float64        `json:"right_drag_raw_travel"`
	DragShare          *float64       `json:"drag_share,omitempty"`
}

type activityTracker struct {
	profile      activityProfile
	held         map[string]bool
	hasMove      bool
	lastMove     int64
	inSegment    bool
	segmentStart int64
}

func newActivityTracker(keyboard bool) *activityTracker {
	counts := make(map[string]int, len(activityKinds))
	for _, kind := range activityKinds {
		counts[kind] = 0
	}
	return &activityTracker{
		profile: activityProfile{Counts: counts, KeyboardCaptured: keyboard},
		held:    make(map[string]bool),
	}
}

func (a *activityTracker) observe(e playtrace.Event) {
	switch e.Type {
	case "segment_start":
		a.inSegment, a.segmentStart = true, e.ElapsedUS
		a.clearMovement()
	case "segment_end":
		if a.inSegment {
			a.profile.ClosedCaptureUS += e.ElapsedUS - a.segmentStart
		}
		a.inSegment = false
		a.clearMovement()
	case "mouse_gap":
		a.clearMovement()
	case "button_down":
		a.held[e.Button] = true
		a.hasMove = false
	case "button_up":
		delete(a.held, e.Button)
	case "move":
		if len(a.held) == 0 {
			if !a.hasMove || e.ElapsedUS-a.lastMove >= movementGapUS {
				a.profile.Counts["free_move_burst"]++
			}
			a.hasMove, a.lastMove = true, e.ElapsedUS
		}
	case "click":
		a.profile.Counts["click"]++
	case "drag":
		a.profile.Counts["drag"]++
		right := e.Button == "right"
		if right {
			a.profile.RightDrags++
		}
		if e.Gesture != nil {
			a.profile.DragRawTravel += max(0, e.Gesture.RawTravel)
			if right {
				a.profile.RightDragHoldUS += max(0, e.Gesture.DurationUS)
				a.profile.RightDragRawTravel += max(0, e.Gesture.RawTravel)
			}
		}
	case "key_down":
		if e.Key == nil || e.Key.Repeat || modifierKey(e.Key.VK) {
			return
		}
		kind := "key"
		if e.Key.Chord != "" {
			kind = "shortcut"
		}
		a.profile.Counts[kind]++
	case "wheel", "wheel_horizontal":
		a.profile.Counts["wheel"]++
	}
}

func (a *activityTracker) clearMovement() {
	clear(a.held)
	a.hasMove = false
}

func (a *activityTracker) summarize(schema int, complete bool) *activityProfile {
	p := a.profile
	for _, kind := range activityKinds {
		p.Actions += p.Counts[kind]
	}
	// Older recordings have button transitions but lack classified gestures.
	if schema < 2 || p.Actions == 0 {
		return &p
	}
	var entropy float64
	for _, kind := range activityKinds {
		if n := p.Counts[kind]; n > 0 {
			fraction := float64(n) / float64(p.Actions)
			entropy -= fraction * math.Log(fraction)
		}
	}
	score := 100 * entropy / math.Log(float64(len(activityKinds)))
	p.MixScore = &score
	if complete && !a.inSegment && p.ClosedCaptureUS > 0 {
		rate := float64(p.Actions) * 60_000_000 / float64(p.ClosedCaptureUS)
		p.ActionsPerMinute = &rate
	}
	if gestures := p.Counts["click"] + p.Counts["drag"]; gestures > 0 {
		share := float64(p.Counts["drag"]) / float64(gestures)
		p.DragShare = &share
	}
	return &p
}

func modifierKey(vk uint32) bool {
	return vk >= 0x10 && vk <= 0x12 || vk >= 0xA0 && vk <= 0xA5 || vk == 0x5B || vk == 0x5C
}
