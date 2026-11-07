package main

import (
	"math"
	"testing"

	"github.com/soocke/pixel-bot-go/internal/playtrace"
)

func TestManualActivityMixAndGestureAccounting(t *testing.T) {
	tracker := newActivityTracker(true)
	events := []playtrace.Event{
		{Type: "segment_start"},
		{Type: "move", ElapsedUS: 1},
		{Type: "move", ElapsedUS: 2},
		{Type: "button_down", ElapsedUS: 3, Button: "right"},
		{Type: "move", ElapsedUS: 4},
		{Type: "button_up", ElapsedUS: 5, Button: "right"},
		{Type: "drag", ElapsedUS: 5, Button: "right", Gesture: &playtrace.Gesture{DurationUS: 2, RawTravel: 30}},
		{Type: "click", ElapsedUS: 6},
		{Type: "key_down", ElapsedUS: 7, Key: &playtrace.Key{VK: 0xA0}},
		{Type: "key_down", ElapsedUS: 8, Key: &playtrace.Key{VK: 'B', Chord: "Shift+B"}},
		{Type: "key_down", ElapsedUS: 9, Key: &playtrace.Key{VK: 'B', Chord: "Shift+B", Repeat: true}},
		{Type: "key_down", ElapsedUS: 10, Key: &playtrace.Key{VK: 'A'}},
		{Type: "wheel_horizontal", ElapsedUS: 11},
		{Type: "segment_end", ElapsedUS: 60_000_000},
	}
	for _, event := range events {
		tracker.observe(event)
	}
	got := tracker.summarize(2, true)
	for _, kind := range activityKinds {
		if got.Counts[kind] != 1 {
			t.Errorf("%s count = %d, want 1", kind, got.Counts[kind])
		}
	}
	if got.MixScore == nil || math.Abs(*got.MixScore-100) > 1e-9 || got.Actions != 6 ||
		got.RightDrags != 1 || got.RightDragHoldUS != 2 || got.RightDragRawTravel != 30 ||
		got.DragShare == nil || *got.DragShare != .5 || got.ActionsPerMinute == nil || *got.ActionsPerMinute != 6 {
		t.Fatalf("unexpected activity profile: %+v", got)
	}
	if legacy := tracker.summarize(1, true); legacy.MixScore != nil {
		t.Fatal("a schema without gesture labels received a mix score")
	}
	if incomplete := tracker.summarize(2, false); incomplete.ActionsPerMinute != nil {
		t.Fatal("an incomplete trace received a capture-normalized rate")
	}
}

func TestMovementBurstsSplitOnIdleAndCaptureGaps(t *testing.T) {
	tracker := newActivityTracker(false)
	for _, event := range []playtrace.Event{
		{Type: "move", ElapsedUS: 1},
		{Type: "move", ElapsedUS: 100},
		{Type: "move", ElapsedUS: 100 + movementGapUS},
		{Type: "mouse_gap", ElapsedUS: 101 + movementGapUS},
		{Type: "move", ElapsedUS: 102 + movementGapUS},
	} {
		tracker.observe(event)
	}
	got := tracker.summarize(2, false)
	if got.Counts["free_move_burst"] != 3 || got.MixScore == nil || *got.MixScore != 0 {
		t.Fatalf("unexpected activity profile: %+v", got)
	}
}
