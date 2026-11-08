package recorder

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/internal/playtrace"
)

func TestRightDragUsesRawMovementWithStationaryCursor(t *testing.T) {
	var tracker gestureTracker
	var got []inputEvent
	emit := func(event inputEvent) { got = append(got, event) }
	point := clientPosition{X: 40, Y: 80, Width: 200, Height: 200}
	tracker.packet(rawMouse{ButtonFlags: 1 << 2}, point, 100, 1, emit)
	tracker.packet(rawMouse{DX: 12, DY: -4}, point, 200, 1, emit)
	tracker.packet(rawMouse{DX: -12, DY: 4}, point, 300, 1, emit)
	tracker.packet(rawMouse{ButtonFlags: 2 << 2}, point, 400, 1, emit)
	event := got[len(got)-1]
	if event.Type != "drag" || event.Button != "right" || event.Gesture.DurationUS != 300 ||
		event.Gesture.MaxClientPixels != 0 || event.Gesture.RawDX != 0 || event.Gesture.RawDY != 0 ||
		event.Gesture.MaxRawCounts < dragCounts || got[1].DX != 12 || got[1].DY != -4 {
		t.Fatalf("stationary cursor with raw movement should be a right drag: %+v, %+v", event, event.Gesture)
	}
}

func TestSideButtonClicksAndInterruptedDrag(t *testing.T) {
	var tracker gestureTracker
	var got []inputEvent
	emit := func(event inputEvent) { got = append(got, event) }
	point := clientPosition{X: 10, Y: 10, Width: 100, Height: 100}
	for i := 3; i < 5; i++ {
		tracker.packet(rawMouse{ButtonFlags: 1 << (i * 2)}, point, 100, 1, emit)
		tracker.packet(rawMouse{ButtonFlags: 2 << (i * 2)}, point, 150, 1, emit)
		if event := got[len(got)-1]; event.Type != "click" || event.Button != mouseButtons[i] || event.Gesture.DurationUS != 50 {
			t.Fatalf("expected side button click, got %+v", event)
		}
	}
	tracker.packet(rawMouse{ButtonFlags: 1 << 2}, point, 200, 1, emit)
	tracker.leave(300, 1, emit)
	if got[len(got)-2].Type != "gesture_interrupted" || got[len(got)-1].Type != "mouse_gap" {
		t.Fatalf("leaving client must interrupt the held button: %+v", got)
	}
}

func TestKeyboardTracksChordsAndRepeats(t *testing.T) {
	var keys keyboardTracker
	keys.event(keyboardData{VK: 0xA0, Scan: 0x2A}, true, 1, 1)
	b := keyboardData{VK: 'B', Scan: 0x30}
	first := keys.event(b, true, 2, 1)
	repeat := keys.event(b, true, 3, 1)
	up := keys.event(b, false, 4, 1)
	keys.clear()
	plain := keys.event(b, true, 5, 2)
	if first.Key.Chord != "Shift+B" || first.Key.Scan != 0x30 || first.Key.Repeat ||
		repeat.Key.Chord != "Shift+B" || !repeat.Key.Repeat || up.Type != "key_up" ||
		plain.Key.Chord != "" || plain.Key.Repeat {
		t.Fatalf("unexpected keyboard state: %+v %+v %+v %+v", first.Key, repeat.Key, up.Key, plain.Key)
	}
}

func TestScopeBreaksStartNewSegments(t *testing.T) {
	var tracker segmentTracker
	var got []inputEvent
	for i, eligible := range []bool{false, true, true, false, false, true, false} {
		tracker.change(eligible, int64(i), func(event inputEvent) { got = append(got, event) })
	}
	want := []inputEvent{
		{Type: "segment_start", ElapsedUS: 1, Segment: 1},
		{Type: "segment_end", ElapsedUS: 3, Segment: 1},
		{Type: "segment_start", ElapsedUS: 5, Segment: 2},
		{Type: "segment_end", ElapsedUS: 6, Segment: 2},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("segments = %+v, want %+v", got, want)
	}
}

type cancelOnArmed struct {
	cancel context.CancelFunc
}

func (w cancelOnArmed) Write(p []byte) (int, error) {
	w.cancel()
	return len(p), nil
}

func TestRecorderStopsAndFinalizesWithoutOverwriting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := filepath.Join(t.TempDir(), "session.jsonl")
	// Cancel once the input listeners are installed. An empty handle has no eligible
	// client area, so this test never records desktop activity.
	if err := run(ctx, windowTarget{}, out, cancelOnArmed{cancel}, false, true, "test-session", time.Now()); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var start playtrace.Header
	if err := dec.Decode(&start); err != nil {
		t.Fatal(err)
	}
	var end playtrace.End
	if err := dec.Decode(&end); err != nil {
		t.Fatal(err)
	}
	if start.Type != "session_start" || start.Schema != playtrace.Schema || start.Source != playtrace.Manual || !start.Keyboard || start.StartedUTC == nil || start.StartedUTC.IsZero() ||
		end.Type != "session_end" || end.Status != "complete" || end.Moves != 0 ||
		end.ButtonEvents != 0 || end.Segments != 0 || dec.More() {
		t.Fatalf("unexpected session file: %s", raw)
	}
	if err := run(ctx, windowTarget{}, out, cancelOnArmed{cancel}, false, true, "test-session", time.Now()); !os.IsExist(err) {
		t.Fatalf("expected existing output to be refused, got %v", err)
	}
	unchanged, err := os.ReadFile(out)
	if err != nil || !bytes.Equal(raw, unchanged) {
		t.Fatalf("original trace changed after rejected overwrite: %v", err)
	}
}
